package rabbitmq

import (
	"io"

	"github.com/rabbitmq/amqp091-go"
)

// the file contains interfaces for the base amqp091 library, this is so we can easily override in tests, and it also
// limits the functionality to what we need.

var (
	dialConfig = amqp091.DialConfig // dialConfig is the dialer function to use to connect to amqp091 with config
	dial       = amqp091.Dial       // dial is the dialer function to use to connect to amqp091
)

// see: github.com/rabbitmq/amqp091-go/channel.go
type amqp091Channel interface {
	io.Closer
	IsClosed() bool
	Qos(count, size int, global bool) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
	QueueBind(queue, routingKey, exchange string, noWait bool, args amqp091.Table) error
	ExchangeDeclare(name, typ string, durable, autoDelete, internal, noWait bool, args amqp091.Table) error
	Publish(exchange, routingKey string, mandatory, immediate bool, msg amqp091.Publishing) error
	Cancel(consumerName string, noWait bool) error
	NotifyClose(rcv chan *amqp091.Error) chan *amqp091.Error
	Consume(
		queue, consumerName string,
		autoAck, exclusive, noLocal, noWait bool,
		args amqp091.Table,
	) (<-chan amqp091.Delivery, error)
}

// see: github.com/rabbitmq/amqp091-go/connection.go
type amqp091Connection interface {
	io.Closer
	IsClosed() bool
	Channel() (*amqp091.Channel, error)
	NotifyClose(rcv chan *amqp091.Error) chan *amqp091.Error
}
