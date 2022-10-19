package amqp

import (
	"context"
	"io"
)

// ExchangeType represents a type of exchange.
type ExchangeType string

const (
	// ExchangeTypeDirect represents a direct exchange
	// this is where a message is posted to bound queues where the routing key matches exactly.
	ExchangeTypeDirect ExchangeType = "direct"
	// ExchangeTypeFanout represents a fanout exchange
	// this is where the routing key is ignored and all bound queues receive a copy of the message.
	ExchangeTypeFanout ExchangeType = "fanout"
	// ExchangeTypeTopic represents a topic exchange
	// this extends on top of a direct exchange by allowing the routing key to be pattern based rather
	// than having to match exactly.
	ExchangeTypeTopic ExchangeType = "topic"
	// ExchangeTypeHeaders represents a headers exchange
	// this is where one or more headers are used to route the message
	ExchangeTypeHeaders ExchangeType = "headers"
)

// ErrorNotificationFunc the callback function type which receives
// errors from the server.
type ErrorNotificationFunc = func(e Error)

// CancelFunc a function which can be used to cancel an active consume
// this is safe to be called concurrently.
type CancelFunc = func()

// Channel represents a single AMQP channel.
//
// A channel is described as a lightweight connection which can be spawned from a single TCP connection
// source. We can have n number of channels on the same Connection. All AMQP operations are derived from a Channel
// rather than the connection itself.
type Channel interface {
	io.Closer
	Notifier

	// QoS sets the prefetch count and size on a channel. This affectively limits how many messages
	// that can be passed to a consumer to process at once. For example if the count is 5 then after
	// the consumer has received 5 messages, the queue won't deliver any more until the consumer has ack'd
	// any of the messages it is currently processing.
	//
	// count is the amount of messages a consumer can be processing, size is the amount of bytes (message size) a
	// consumer can be processing, and global changes whether it affects a single consumer or ALL consumers
	// bound to a channel.
	QoS(ctx context.Context, count, size int64, global bool) error
	// CreateQueue attempts to declare a queue, returning the generated queue to use.
	CreateQueue(ctx context.Context, name string, durable, autoDelete, exclusive bool) (Queue, error)
	// BindQueue attempts to bind a queue to an exchange.
	BindQueue(ctx context.Context, queue, exchange, routingKey string) error
	// CreateExchange attempts to declare an exchange.
	CreateExchange(ctx context.Context, name string, typ ExchangeType, durable, autoDelete bool) error
	// Publish attempts to publish a message to an exchange using the supplied routing key
	// if the exchange is empty, the default exchange defined by the broker will be used
	// which affectively routes the message a queue with exactly the same name as the routing key.
	Publish(ctx context.Context, exchange, routingKey string, body io.Reader) error
	// Consume starts consuming messages from a queue, inbound messages are send to the returned channel.
	Consume(ctx context.Context, queue, consumerName string, autoAck, exclusive bool) (msgs <-chan Message, cancel CancelFunc, err error)
	// IsClosed determines if the connection is closed.
	IsClosed() bool
}
