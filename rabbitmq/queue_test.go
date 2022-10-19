package rabbitmq

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jacklaaa89/amqp"
)

func TestQueue_Bind(t *testing.T) {
	tt := []struct {
		Name     string
		Setup    func(h *mockAMQPChannelHandlers)
		Expected func(t *testing.T, err error)
	}{
		{
			Name: "Valid",
			Setup: func(h *mockAMQPChannelHandlers) {
				h.QueueBind = func() error {
					return nil
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			Name: "ErrFromAMQP",
			Setup: func(h *mockAMQPChannelHandlers) {
				h.QueueBind = func() error {
					return errors.New("could not bind queue")
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			h := newDefaultAMQPChannelHandlers()
			tc.Setup(&h)
			ch := &channel{Channel: &mockAMQPChannel{h: h}}
			err := (&queue{ch: ch}).Bind(context.Background(), "test", "test")
			tc.Expected(t, err)
		})
	}
}

func TestQueue_Consume(t *testing.T) {
	closed, cancel := context.WithCancel(context.Background())
	cancel() // cancel straight away.

	tt := []struct {
		Name     string
		Context  context.Context
		Setup    func(h *mockAMQPChannelHandlers)
		Expected func(t *testing.T, ch <-chan amqp.Message, err error)
	}{
		{
			Name: "Valid",
			Setup: func(h *mockAMQPChannelHandlers) {
				h.Consume = func() (<-chan amqp091.Delivery, error) {
					ch := make(chan amqp091.Delivery, 1)
					ch <- amqp091.Delivery{Body: []byte("12345")}
					// the consumer doesn't see it as the end on a closed delivery channel
					// as this may be intermittent with a reconnection etc.
					// closing the notifyReconnect handler is the way to stop listening.
					close(ch)
					return ch, nil
				}

				// don't close or send a message as this is seen as a channel close.
				h.NotifyClose = func(ch chan *amqp091.Error) chan *amqp091.Error { return ch }
			},
			Expected: func(t *testing.T, ch <-chan amqp.Message, err error) {
				t.SkipNow()

				require.NotNil(t, ch)
				assert.NoError(t, err)

				// read the only pending message
				v, ok := <-ch
				assert.True(t, ok)
				require.NotNil(t, v)
				r, rErr := io.ReadAll(v.Body())
				assert.NoError(t, rErr)
				assert.Equal(t, []byte("12345"), r)

				// assert single message
				_, ok = <-ch
				assert.False(t, ok)
			},
		},
		{
			Name: "ErrFromAMQP",
			Setup: func(h *mockAMQPChannelHandlers) {
				h.Consume = func() (<-chan amqp091.Delivery, error) {
					return nil, errors.New("could not consume from queue")
				}
				// don't close or send a message as this is seen as a channel close.
				h.NotifyClose = func(ch chan *amqp091.Error) chan *amqp091.Error { return ch }
			},
			Expected: func(t *testing.T, ch <-chan amqp.Message, err error) {
				assert.Error(t, err)
			},
		},
		{
			Name:    "ClosedContext",
			Context: closed,
			Setup: func(h *mockAMQPChannelHandlers) {
				ch := make(chan amqp091.Delivery, 1)
				h.Consume = func() (<-chan amqp091.Delivery, error) {
					// this is purposely not closed here.
					return ch, nil
				}
				h.Cancel = func() error {
					close(ch) // this will ensure we attempt to call cancel on the parent.
					return nil
				}
				// don't close or send a message as this is seen as a channel close.
				h.NotifyClose = func(ch chan *amqp091.Error) chan *amqp091.Error { return ch }
			},
			Expected: func(t *testing.T, ch <-chan amqp.Message, err error) {
				// expect no error from consume, but expect the supplied channel
				// to be closed.
				_, ok := <-ch
				assert.False(t, ok)
				assert.NoError(t, err)
			},
		},
		{
			Name:    "ErrFromCancel",
			Context: closed,
			Setup: func(h *mockAMQPChannelHandlers) {
				ch := make(chan amqp091.Delivery, 1)
				h.Consume = func() (<-chan amqp091.Delivery, error) {
					// this is purposely not closed here.
					return ch, nil
				}
				h.Cancel = func() error {
					close(ch) // this will ensure we attempt to call cancel on the parent.
					return errors.New("could not cancel consume")
				}
				// don't close or send a message as this is seen as a channel close.
				h.NotifyClose = func(ch chan *amqp091.Error) chan *amqp091.Error { return ch }
			},
			Expected: func(t *testing.T, ch <-chan amqp.Message, err error) {
				// expect no error from consume, but expect the supplied channel
				// to be closed.
				_, ok := <-ch
				assert.False(t, ok)

				// we can assert that the error from cancel is logged but
				// not returned, as it is more of a courtesy than a requirement.
				assert.NoError(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			if tc.Context != nil {
				ctx = tc.Context
			}

			h := newDefaultAMQPChannelHandlers()
			tc.Setup(&h)
			ch := &channel{Channel: &mockAMQPChannel{h: h}}
			require.NoError(t, ch.init())
			d, stop, err := (&queue{ch: ch}).Consume(ctx, "test", false, false)
			stop()
			tc.Expected(t, d, err)
		})
	}
}

func TestQueue_Name(t *testing.T) {
	assert.Equal(t, "Name", (&queue{Queue: amqp091.Queue{Name: "Name"}}).Name())
}

type errorFunc func() error

type mockAMQPChannelHandlers struct {
	Close           errorFunc
	Qos             errorFunc
	QueueBind       errorFunc
	ExchangeDeclare errorFunc
	Publish         errorFunc
	Cancel          errorFunc
	IsClosed        func() bool
	QueueDeclare    func() (amqp091.Queue, error)
	Consume         func() (<-chan amqp091.Delivery, error)
	NotifyClose     func(ch chan *amqp091.Error) chan *amqp091.Error
}

// newDefaultAMQPChannelHandlers generates a default set of handlers.
func newDefaultAMQPChannelHandlers() mockAMQPChannelHandlers {
	return mockAMQPChannelHandlers{
		Close:           func() error { return nil },
		Qos:             func() error { return nil },
		QueueBind:       func() error { return nil },
		ExchangeDeclare: func() error { return nil },
		Cancel:          func() error { return nil },
		Publish:         func() error { return nil },
		IsClosed:        func() bool { return false },
		QueueDeclare:    func() (amqp091.Queue, error) { return amqp091.Queue{}, nil },
		Consume: func() (<-chan amqp091.Delivery, error) {
			ch := make(chan amqp091.Delivery)
			close(ch)
			return ch, nil
		},
		NotifyClose: func(ch chan *amqp091.Error) chan *amqp091.Error {
			close(ch)
			return ch
		},
	}
}

type mockAMQPChannel struct {
	h mockAMQPChannelHandlers
}

func (m *mockAMQPChannel) Close() error {
	return m.h.Close()
}
func (m *mockAMQPChannel) IsClosed() bool {
	return m.h.IsClosed()
}
func (m *mockAMQPChannel) Qos(_, _ int, _ bool) error {
	return m.h.Qos()
}
func (m *mockAMQPChannel) QueueDeclare(_ string, _, _, _, _ bool, _ amqp091.Table) (amqp091.Queue, error) {
	return m.h.QueueDeclare()
}
func (m *mockAMQPChannel) QueueBind(_, _, _ string, _ bool, _ amqp091.Table) error {
	return m.h.QueueBind()
}
func (m *mockAMQPChannel) ExchangeDeclare(_, _ string, _, _, _, _ bool, _ amqp091.Table) error {
	return m.h.ExchangeDeclare()
}
func (m *mockAMQPChannel) Publish(_, _ string, _, _ bool, _ amqp091.Publishing) error {
	return m.h.Publish()
}
func (m *mockAMQPChannel) Consume(_, _ string, _, _, _, _ bool, _ amqp091.Table) (<-chan amqp091.Delivery, error) {
	return m.h.Consume()
}
func (m *mockAMQPChannel) Cancel(_ string, _ bool) error {
	return m.h.Cancel()
}
func (m *mockAMQPChannel) NotifyClose(rcv chan *amqp091.Error) chan *amqp091.Error {
	return m.h.NotifyClose(rcv)
}

type mockAMQPAcknowledgerHandlers struct {
	Ack    errorFunc
	Nack   errorFunc
	Reject errorFunc
}

// newDefaultAMQPAcknowledgerHandlers generates a default set of handlers.
func newDefaultAMQPAcknowledgerHandlers() mockAMQPAcknowledgerHandlers {
	return mockAMQPAcknowledgerHandlers{
		Ack:    func() error { return nil },
		Nack:   func() error { return nil },
		Reject: func() error { return nil },
	}
}

type mockAMQPAcknowledger struct {
	h mockAMQPAcknowledgerHandlers
}

func (m *mockAMQPAcknowledger) Ack(_ uint64, _ bool) error {
	return m.h.Ack()
}
func (m *mockAMQPAcknowledger) Nack(_ uint64, _, _ bool) error {
	return m.h.Nack()
}
func (m *mockAMQPAcknowledger) Reject(_ uint64, _ bool) error {
	return m.h.Reject()
}

type mockAMQPConnectionHandlers struct {
	Close       errorFunc
	IsClosed    func() bool
	Channel     func() (*amqp091.Channel, error)
	NotifyClose func(ch chan *amqp091.Error) chan *amqp091.Error
}

// newDefaultAMQPConnectionHandlers generates a default set of handlers.
func newDefaultAMQPConnectionHandlers() mockAMQPConnectionHandlers {
	return mockAMQPConnectionHandlers{
		Close:    func() error { return nil },
		IsClosed: func() bool { return false },
		Channel: func() (*amqp091.Channel, error) {
			return &amqp091.Channel{}, nil
		},
		NotifyClose: func(ch chan *amqp091.Error) chan *amqp091.Error {
			close(ch)
			return ch
		},
	}
}

type mockAMQPConnection struct {
	h mockAMQPConnectionHandlers
}

func (m *mockAMQPConnection) Close() error {
	return m.h.Close()
}
func (m *mockAMQPConnection) IsClosed() bool {
	return m.h.IsClosed()
}
func (m *mockAMQPConnection) Channel() (*amqp091.Channel, error) {
	return m.h.Channel()
}
func (m *mockAMQPConnection) NotifyClose(rcv chan *amqp091.Error) chan *amqp091.Error {
	return m.h.NotifyClose(rcv)
}
