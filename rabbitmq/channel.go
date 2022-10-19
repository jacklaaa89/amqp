package rabbitmq

import (
	"context"
	"io"
	"sync"

	"github.com/cenkalti/backoff/v4"
	"github.com/gabriel-vasile/mimetype"
	"github.com/rabbitmq/amqp091-go"

	"github.com/jacklaaa89/amqp"
)

// omitter events.
const (
	reconnection = iota // triggered on reconnection.
)

// channel represents a wrapped amqp091.Channel
type channel struct {
	mu       sync.RWMutex    // mu a guarding mutex for the internal channel.
	reconnMu sync.Mutex      // reconnMu mutex specifically for reconnection attempts.
	omitMu   sync.RWMutex    // mutex for events.
	ctx      context.Context // the server bound context.
	conn     *connection     // the connection which is managing this channel.

	// containers for assigned event handlers.
	closeOnce  sync.Once
	closes     []func()
	reconnects []func()

	// closed represents whether we have called closed specifically on our channel
	// i.e. we have gracefully called close, on the connection has gracefully been called close.
	closed bool

	// consumerNotifications a set of channels which are notified on reconnection.
	// this is stored as a slice so a consumer registers when we start consuming and
	// de-registers when cancelled.
	//
	// this is so consumers don't receive stale reconnection notifications.
	consumerNotifications map[string]chan int

	Channel amqp091Channel // the currently active channel connection.
}

// QoS attempts to set the prefetch count and size for a consumer.
func (c *channel) QoS(ctx context.Context, count, size int64, global bool) error {
	return c.onChannel(ctx, func(ch amqp091Channel) error {
		return ch.Qos(int(count), int(size), global)
	})
}

// CreateQueue attempts to create a new queue.
func (c *channel) CreateQueue(ctx context.Context, name string, durable, autoDelete, exclusive bool) (amqp.Queue, error) {
	var q amqp091.Queue
	err := c.onChannel(ctx, func(ch amqp091Channel) error {
		var qErr error
		q, qErr = ch.QueueDeclare(name, durable, autoDelete, exclusive, false, nil)
		return qErr
	})
	return &queue{q, c}, err
}

// BindQueue attempts to bind a queue to an exchange.
func (c *channel) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	return c.onChannel(ctx, func(ch amqp091Channel) error {
		return ch.QueueBind(queue, routingKey, exchange, false, nil)
	})
}

// CreateExchange attempts to create a new exchange.
func (c *channel) CreateExchange(
	ctx context.Context,
	name string,
	typ amqp.ExchangeType,
	durable, autoDelete bool,
) error {
	return c.onChannel(ctx, func(ch amqp091Channel) error {
		return ch.ExchangeDeclare(name, string(typ), durable, autoDelete, false, false, nil)
	})
}

// Publish attempts to publish a message onto an exchange with the supplied routing key.
func (c *channel) Publish(ctx context.Context, exchange, routingKey string, body io.Reader) error {
	b, err := io.ReadAll(body)
	if err != nil {
		logError(ctx, err)
		return err
	}

	m := mimetype.Detect(b)
	return c.onChannel(ctx, func(ch amqp091Channel) error {
		return ch.Publish(exchange, routingKey, true, false, amqp091.Publishing{
			ContentType: m.String(),
			Body:        b,
		})
	})
}

// NotifyClose registers a handler to be triggered on a close.
func (c *channel) NotifyClose(fn func()) {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	if fn == nil {
		return
	}

	c.closes = append(c.closes, fn)
}

// NotifyReconnect registers a handler to be triggered on a successful reconnect.
func (c *channel) NotifyReconnect(fn func()) {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	if fn == nil {
		return
	}

	c.reconnects = append(c.reconnects, fn)
}

// Close wraps the original close function.
func (c *channel) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return amqp091.ErrClosed
	}

	c.closed = true
	c.mu.Unlock()

	c.closeActiveConsumes()
	go c.closeOnce.Do(c.omitClose)
	return c.Channel.Close()
}

// IsClosed wraps the original IsClosed function.
func (c *channel) IsClosed() bool {
	if c == nil {
		return true
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return true
	}

	return isClosed(c.Channel)
}

// Consume attempts to consume from a queue, this function
// differs slightly from the original SDK's implementation where
// this function will also take the supplied context into
// consideration when consuming messages.
func (c *channel) Consume(
	ctx context.Context,
	queue, consumerName string,
	autoAck, exclusive bool,
) (messages <-chan amqp.Message, cancel amqp.CancelFunc, err error) {
	msgs := make(chan amqp.Message)
	cancel, err = c.consume(ctx, queue, consumerName, autoAck, exclusive, msgs)
	return msgs, cancel, err
}

// onChannel helper function to perform an action on the raw amqp091.Channel
func (c *channel) onChannel(ctx context.Context, fn func(ch amqp091Channel) error) error {
	if c.IsClosed() {
		// attempt to reconnect.
		if err := c.reconnect(); err != nil {
			return err
		}
	}

	c.mu.RLock()
	err := fn(c.Channel)
	c.mu.RUnlock()

	if err != nil {
		logError(ctx, err)
	}
	return err
}

// reconnect attempts to generate a new channel connection when
// it is safe to recover from a close.
// because we use the underlined connection which would have also been made
// aware of a close, this function should be able to recover from connection
// level closes as well.
func (c *channel) reconnect() error {
	// only allow a single thread to perform reconnect at once.
	c.reconnMu.Lock()
	defer c.reconnMu.Unlock()

	if !c.IsClosed() {
		return nil
	}

	var closed bool
	c.mu.RLock()
	closed = c.closed
	c.mu.RUnlock()

	if closed {
		return amqp091.ErrClosed
	}

	err := backoff.Retry(func() error {
		ch, err := c.conn.rawChannel()
		if err != nil {
			return err
		}

		c.mu.Lock()
		c.Channel = ch
		c.mu.Unlock()
		return c.init()
	}, newBackoff(c.ctx))

	if err != nil {
		logError(c.ctx, c.Close())
		return err
	}

	// inform all consumers of the reconnection.
	// this should never block as we should only ever
	// have listening consumers in the map
	// however to be safe this is done in a separate routine to
	// not block the reconnection cycle.
	go c.signalConsumers(reconnection)

	// inform all listeners about the successful reconnection.
	// this is also done in a separate routine for the same reasons
	// as above.
	go c.omitReconnect()

	return nil
}

// omitReconnect omits a reconnect event to all handlers.
// this is triggered everytime a successful reconnection is made
func (c *channel) omitReconnect() {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	for _, fn := range c.reconnects {
		fn()
	}
}

// omitClose omits a close event to all handlers.
// close will ever be omitted once
// a channel can be gracefully closed (by the SDK) once
// after that it cannot be reused.
// this triggers on a failed reconnect or if we call Close directly.
func (c *channel) omitClose() {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	for _, fn := range c.closes {
		fn()
	}
}

// represents a function which does nothing.
var emptyFunc = func() {
	// intentionally empty to stop calling a nil function
	// to cancel the consume process on error.
}

// consume starts consuming on a queue, pushing new deliveries to the supplied ch.
func (c *channel) consume(
	ctx context.Context,
	queue, consumerName string,
	autoAck, exclusive bool,
	ch chan<- amqp.Message,
) (amqp.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	active, err := c.startConsume(ctx, queue, consumerName, autoAck, exclusive)
	if err != nil {
		cancel()
		return emptyFunc, err
	}

	go func() {
		notifications := c.registerConsume(consumerName)

		var finish = func() {
			logError(ctx, active.cancel())
			c.deregisterConsume(consumerName)
			cancel()
			close(ch)
		}

	forever:
		for {
			select {
			case <-ctx.Done():
				// the context is for the consuming context only
				// it doesn't control the entire channel connection.
				finish()
				return
			case _, ok := <-notifications:
				// if we receive a message that we have reconnected
				// restart consuming
				// if ok == false, then assume no more reconnections will be made.
				if !ok {
					finish()
					return
				}
				break forever
			case d, ok := <-active.deliveries:
				if !ok {
					continue forever // awaiting reconnect.
				}
				// on each delivery, push down awaiting channel.
				ch <- &message{ctx, d}
			}
		}

		// ensure we clean up the now dead consume.
		logError(ctx, active.cancel())

		// we can ignore the cancel function spawned from sub-consumes
		// as this will still be bound by the original ctx which started the consume
		// process at the beginning, as this is a recursive call.
		_, err = c.consume(ctx, queue, consumerName, autoAck, exclusive, ch)
		logError(ctx, err)
	}()

	return cancel, nil
}

// activeConsume represents a low-level active consume which is
// happening on a rabbitmq channel.
//
// having this as a struct allows us to override the current consume
// variables at runtime.
type activeConsume struct {
	deliveries <-chan amqp091.Delivery // deliveries a channel where queue deliveries are posted.
	cancel     func() error            // cancel a function to cancel the consume process.
}

// startConsume attempts to start consuming from a queue using the channel, returning a reference to the
// active consume.
func (c *channel) startConsume(ctx context.Context, queue, consumerName string, autoAck, exclusive bool) (*activeConsume, error) {
	var dc <-chan amqp091.Delivery
	err := c.onChannel(ctx, func(ch amqp091Channel) error {
		var cErr error
		dc, cErr = ch.Consume(queue, consumerName, autoAck, exclusive, false, false, nil)
		return cErr
	})

	if err != nil {
		return nil, err
	}

	return &activeConsume{
		deliveries: dc,
		cancel: func() error {
			return c.onChannel(ctx, func(ch amqp091Channel) error {
				return ch.Cancel(consumerName, false)
			})
		},
	}, nil
}

// init initialises a channel to listen for closes.
func (c *channel) init() error {
	c.mu.Lock()
	if c.consumerNotifications == nil {
		c.consumerNotifications = make(map[string]chan int)
	}
	c.mu.Unlock()

	rcv := make(chan *amqp091.Error)
	err := c.onChannel(c.ctx, func(ch amqp091Channel) error {
		ch.NotifyClose(rcv)
		return nil
	})

	if err != nil {
		return err
	}

	go func() {
		e, ok := <-rcv
		// handle graceful closes.
		if !ok || e == nil {
			// Close will also handle gracefully stopping any
			// active consumes.
			logError(c.ctx, c.Close())
			return
		}
		// attempt to reconnect on any other error.
		logError(c.ctx, c.reconnect())
	}()

	return nil
}

// registerConsume registers a consumer on the channel under the consumer name.
// if we already have a consumer of the same name, the original channel is returned
// the consumer name should be unique per consume process.
func (c *channel) registerConsume(consumerName string) <-chan int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.consumerNotifications[consumerName]; ok {
		return v
	}

	ch := make(chan int)
	c.consumerNotifications[consumerName] = ch
	return ch
}

// deregisterConsume removes a consumer from the reconnection notification handler.
func (c *channel) deregisterConsume(consumerName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.consumerNotifications[consumerName]
	if !ok {
		return
	}

	delete(c.consumerNotifications, consumerName)
	close(ch)
}

// signalConsumers signal to consumers that a reconnection has been completed.
func (c *channel) signalConsumers(e int) {
	var consumers map[string]chan int
	c.mu.Lock()
	consumers = c.consumerNotifications
	c.mu.Unlock()

	for _, consumer := range consumers {
		consumer <- e
	}
}

// closeActiveConsumes triggers a close in the
// registered channels which consumers are listening
// on which inform a consumer to stop consuming.
func (c *channel) closeActiveConsumes() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// close active consumers.
	for _, consumer := range c.consumerNotifications {
		close(consumer)
	}

	// reset the map.
	c.consumerNotifications = make(map[string]chan int)
}
