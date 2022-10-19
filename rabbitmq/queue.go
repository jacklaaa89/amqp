package rabbitmq

import (
	"context"

	"github.com/rabbitmq/amqp091-go"

	"github.com/jacklaaa89/amqp"
)

// queue represents a wrapped amqp091.Queue with additional features.
type queue struct {
	amqp091.Queue
	ch amqp.Channel // ch a channel instance; so we can add some helper functionality to the queue.
}

// Name returns the name of the queue.
func (q *queue) Name() string { return q.Queue.Name }

// Bind attempts to bind this queue to the requested exchange using the routing key.
func (q *queue) Bind(ctx context.Context, exchange, routingKey string) error {
	return q.ch.BindQueue(ctx, q.Name(), exchange, routingKey)
}

// Consume helper function which allows us to consume directly from the defined queue, rather than supplying the
// queue name when calling channel.Consume.
func (q *queue) Consume(ctx context.Context, consumerName string, autoAck, exclusive bool) (msgs <-chan amqp.Message, cancel amqp.CancelFunc, err error) {
	return q.ch.Consume(ctx, q.Name(), consumerName, autoAck, exclusive)
}
