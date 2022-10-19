package rabbitmq

import (
	"context"
	"github.com/jacklaaa89/amqp"
	"sync"

	"github.com/cenkalti/backoff/v4"
	"github.com/rabbitmq/amqp091-go"
)

// helper types exposed from the underlined SDK package.

type (
	Config         = amqp091.Config
	Authentication = amqp091.Authentication
	PlainAuth      = amqp091.PlainAuth
)

// amqp091ConnectionDialer a function which takes no arguments and returns a new amqp091 connection.
type amqp091ConnectionDialer = func() (amqp091Connection, error)

// connection represents an amqp091.Connection which implements our generic implementation.
// it also has fields which allows us to perform reconnects if necessary
type connection struct {
	mu       sync.RWMutex // variable guard.
	reconnMu sync.Mutex   // mutex for reconnections
	omitMu   sync.RWMutex // mutex for events.

	dialer amqp091ConnectionDialer // the function to use to connect to the AMQP client.
	ctx    context.Context         // a server bound context.
	closed bool                    // whether the connection is closed.

	// containers for assigned event handlers.
	closeOnce  sync.Once
	closes     []func()
	reconnects []func()

	Connection amqp091Connection // the connection.
}

// DialConfig attempts to connect to a rabbitmq broker using an amqp:// url while also
// supplying Config to define authentication etc.
func DialConfig(ctx context.Context, addr string, c Config) amqp.Dialer { //nolint // config has to be non-pointer to conform to amqp091.
	return func() (amqp.Connection, error) {
		return wrapDial(ctx, func() (amqp091Connection, error) {
			return dialConfig(addr, c)
		})
	}
}

// Dial attempts to connect to a rabbitmq broker using an amqp:// url.
func Dial(ctx context.Context, addr string) amqp.Dialer {
	return func() (amqp.Connection, error) {
		return wrapDial(ctx, func() (amqp091Connection, error) {
			return dial(addr)
		})
	}
}

// wrapDial helper function to wrap an amqp091.Connection as our generic interface implementation.
func wrapDial(ctx context.Context, dial amqp091ConnectionDialer) (amqp.Connection, error) {
	conn, err := dial()
	if err != nil {
		return nil, err
	}
	c := &connection{
		Connection: conn,
		dialer:     dial,
		ctx:        ctx,
	}
	go c.background()
	return c, nil
}

// Channel initialises a new AMQP channel from a connection.
func (c *connection) Channel() (amqp.Channel, error) {
	ch, err := c.rawChannel()
	if err != nil {
		return nil, err
	}

	wc := &channel{Channel: ch, conn: c, ctx: c.ctx}
	err = wc.init()
	return wc, err
}

// rawChannel returns a lower level channel
func (c *connection) rawChannel() (amqp091Channel, error) {
	var ch amqp091Channel
	err := c.onConnection(func(conn amqp091Connection) error {
		var cErr error
		ch, cErr = conn.Channel()
		return cErr
	})
	return ch, err
}

// NotifyClose registers a handler to be triggered on a close.
func (c *connection) NotifyClose(fn func()) {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	if fn == nil {
		return
	}

	c.closes = append(c.closes, fn)
}

// NotifyReconnect registers a handler to be triggered on a successful reconnect.
func (c *connection) NotifyReconnect(fn func()) {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	if fn == nil {
		return
	}

	c.reconnects = append(c.reconnects, fn)
}

// omitReconnect omits a reconnect event to all handlers.
func (c *connection) omitReconnect() {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	for _, fn := range c.reconnects {
		fn()
	}
}

// omitClose omits a close event to all handlers.
func (c *connection) omitClose() {
	c.omitMu.Lock()
	defer c.omitMu.Unlock()
	for _, fn := range c.closes {
		fn()
	}
}

// Close wraps the original close function.
func (c *connection) Close() error {
	if c.IsClosed() {
		return nil // already closed.
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	go c.closeOnce.Do(c.omitClose)
	return c.Connection.Close()
}

// IsClosed wraps the original IsClosed function.
func (c *connection) IsClosed() bool {
	if c == nil {
		return true
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return true
	}

	return isClosed(c.Connection)
}

// onConnection helper function to perform an action on the raw amqp091.Connection
func (c *connection) onConnection(fn func(conn amqp091Connection) error) error {
	if c.IsClosed() {
		if err := c.reconnect(); err != nil {
			return err
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return fn(c.Connection)
}

// reconnect attempts to reconnect a closed connection
// if it was deemed that the error was recoverable.
func (c *connection) reconnect() error {
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
		conn, err := c.dialer() // use the originally defined dialer to reconnect with.
		if err != nil {
			return err
		}

		c.mu.Lock()
		c.Connection = conn
		c.mu.Unlock()
		return nil
	}, newBackoff(c.ctx))

	if err != nil {
		logError(c.ctx, c.Close())
		return err
	}

	go c.background()
	go c.omitReconnect()
	return nil
}

// background starts any required background functionality for a connection.
// listenForNonRecoverableErrors is a blocking operation; so we cannot wrap it in c.onChannel
// as that would lock the mutex and not allow anything else to perform operations on
// the connection.
func (c *connection) background() {
	ch := make(chan *amqp091.Error)
	err := c.onConnection(func(conn amqp091Connection) error {
		conn.NotifyClose(ch)
		return nil
	})

	if err != nil {
		logError(c.ctx, err)
		return
	}

	select {
	case <-c.ctx.Done():
		logError(c.ctx, c.Close()) // this will also close ch if it's not already closed.
		return
	case e, ok := <-ch:
		if !ok || e == nil {
			logError(c.ctx, c.Close()) // this will also close ch if it's not already closed.
			return
		}
		if rErr := c.reconnect(); rErr != nil {
			logError(c.ctx, rErr)
		}
	}
}
