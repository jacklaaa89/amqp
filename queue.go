package amqp

import (
	"context"
	"io"
)

// Message represents an inbound AMQP message.
type Message interface {
	// Context returns a scoped context for this specific message.
	Context() context.Context
	// Ack attempts to acknowledge that we have processed the message
	Ack() error
	// Nack attempts to acknowledge that we failed processing the message
	Nack(requeue bool) error
	// Body returns the message Body as a reader.
	Body() io.Reader
	// Redelivered whether the message has been redelivered previously.
	Redelivered() bool
}

// Queue represents a single AMQP queue instance.
type Queue interface {
	// Name returns the name of the queue.
	Name() string
	// Bind attempts to bind this queue to an exchange based on the supplied routing key.
	Bind(ctx context.Context, exchange, routingKey string) error
	// Consume starts consuming messages from a queue, inbound messages are send to the returned channel.
	Consume(ctx context.Context, consumerName string, autoAck, exclusive bool) (messages <-chan Message, cancel CancelFunc, err error)
}
