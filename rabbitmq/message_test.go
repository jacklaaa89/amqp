package rabbitmq

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestMessage_Ack(t *testing.T) {
	tt := []struct {
		Name     string
		Setup    func(h *mockAMQPAcknowledgerHandlers)
		Expected func(t *testing.T, err error)
	}{
		{
			Name: "Valid",
			Setup: func(h *mockAMQPAcknowledgerHandlers) {
				h.Ack = func() error {
					return nil
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			Name: "ErrFromAMQP",
			Setup: func(h *mockAMQPAcknowledgerHandlers) {
				h.Ack = func() error {
					return errors.New("could not ack")
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			h := newDefaultAMQPAcknowledgerHandlers()
			tc.Setup(&h)
			msg := &message{Delivery: amqp091.Delivery{Acknowledger: &mockAMQPAcknowledger{h: h}}}
			err := msg.Ack()
			tc.Expected(t, err)
		})
	}
}

func TestMessage_Nack(t *testing.T) {
	tt := []struct {
		Name     string
		Setup    func(h *mockAMQPAcknowledgerHandlers)
		Expected func(t *testing.T, err error)
	}{
		{
			Name: "Valid",
			Setup: func(h *mockAMQPAcknowledgerHandlers) {
				h.Nack = func() error {
					return nil
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			Name: "ErrFromAMQP",
			Setup: func(h *mockAMQPAcknowledgerHandlers) {
				h.Nack = func() error {
					return errors.New("could not nack")
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			h := newDefaultAMQPAcknowledgerHandlers()
			tc.Setup(&h)
			msg := &message{Delivery: amqp091.Delivery{Acknowledger: &mockAMQPAcknowledger{h: h}}}
			err := msg.Nack(false)
			tc.Expected(t, err)
		})
	}
}

func TestMessage_Body(t *testing.T) {
	r := (&message{Delivery: amqp091.Delivery{Body: []byte("test")}}).Body()
	assert.IsType(t, &bytes.Reader{}, r)
	d, err := io.ReadAll(r)
	assert.Equal(t, []byte("test"), d)
	assert.NoError(t, err)
}

func TestMessage_Context(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, ctx, (&message{ctx: ctx}).Context())
}

func TestMessage_Redelivered(t *testing.T) {
	assert.True(t, (&message{Delivery: amqp091.Delivery{Redelivered: true}}).Redelivered())
}
