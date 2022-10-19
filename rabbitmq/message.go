package rabbitmq

import (
	"bytes"
	"context"
	"io"

	"github.com/rabbitmq/amqp091-go"
)

// message implements a AMQP message
type message struct {
	ctx context.Context
	amqp091.Delivery
}

// Context returns the attached ctx
func (m *message) Context() context.Context {
	return m.ctx
}

// Ack attempts to acknowledge a message.
func (m *message) Ack() error {
	return m.Delivery.Ack(false)
}

// Nack attempts to negatively acknowledge a message.
func (m *message) Nack(requeue bool) error {
	return m.Delivery.Nack(false, requeue)
}

// Body returns the message body as an io.Reader
func (m *message) Body() io.Reader {
	return bytes.NewReader(m.Delivery.Body)
}

// Redelivered whether the message has been redelivered previously.
func (m *message) Redelivered() bool {
	return m.Delivery.Redelivered
}
