package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"

	"github.com/jacklaaa89/amqp"
)

func TestDial(t *testing.T) {
	var setupDial = func(dialer func(addr string) (*amqp091.Connection, error)) func() {
		original := dial
		dial = dialer
		return func() {
			dial = original
		}
	}

	tt := []struct {
		Name     string
		Addr     string
		Dialer   func(addr string) (*amqp091.Connection, error)
		Expected func(t *testing.T, con amqp.Connection, err error)
	}{
		{
			Name: "Valid",
			Addr: "amqp://",
			Dialer: func(addr string) (*amqp091.Connection, error) {
				assert.Equal(t, "amqp://", addr)
				return &amqp091.Connection{}, nil
			},
			Expected: func(t *testing.T, con amqp.Connection, err error) {
				assert.IsType(t, &connection{}, con)
				assert.NoError(t, err)
			},
		},
		{
			Name: "ErrFromAMQP",
			Addr: "amqp://",
			Dialer: func(addr string) (*amqp091.Connection, error) {
				assert.Equal(t, "amqp://", addr)
				return nil, errors.New("net: i/o timeout")
			},
			Expected: func(t *testing.T, con amqp.Connection, err error) {
				assert.Nil(t, con)
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			defer setupDial(tc.Dialer)()
			fn := Dial(context.Background(), tc.Addr)
			con, err := fn()
			tc.Expected(t, con, err)
		})
	}
}

func TestDialConfig(t *testing.T) {
	var setupDial = func(dialer func(addr string, c Config) (*amqp091.Connection, error)) func() {
		original := dialConfig
		dialConfig = dialer
		return func() {
			dialConfig = original
		}
	}

	tt := []struct {
		Name     string
		Addr     string
		Username string
		Password string
		Dialer   func(addr string, c Config) (*amqp091.Connection, error)
		Expected func(t *testing.T, con amqp.Connection, err error)
	}{
		{
			Name:     "Valid",
			Addr:     "amqp://",
			Username: "User",
			Password: "Password",
			Dialer: func(addr string, c Config) (*amqp091.Connection, error) {
				assert.Equal(t, "amqp://", addr)
				assert.Len(t, c.SASL, 1)
				auth := c.SASL[0]
				assert.Equal(t, "PLAIN", auth.Mechanism())
				assert.Equal(t, "\x00User\x00Password", auth.Response())
				return &amqp091.Connection{}, nil
			},
			Expected: func(t *testing.T, con amqp.Connection, err error) {
				assert.IsType(t, &connection{}, con)
				assert.NoError(t, err)
			},
		},
		{
			Name:     "ErrFromAMQP",
			Addr:     "amqp://",
			Username: "User",
			Password: "Password",
			Dialer: func(addr string, c Config) (*amqp091.Connection, error) {
				assert.Equal(t, "amqp://", addr)
				assert.Len(t, c.SASL, 1)
				auth := c.SASL[0]
				assert.Equal(t, "PLAIN", auth.Mechanism())
				assert.Equal(t, "\x00User\x00Password", auth.Response())
				return nil, errors.New("net: i/o timeout")
			},
			Expected: func(t *testing.T, con amqp.Connection, err error) {
				assert.Nil(t, con)
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			defer setupDial(tc.Dialer)()

			c := Config{
				SASL: []Authentication{&PlainAuth{
					Username: tc.Username,
					Password: tc.Password,
				}},
			}

			fn := DialConfig(context.Background(), tc.Addr, c)
			con, err := fn()
			tc.Expected(t, con, err)
		})
	}
}

func TestConnection_Channel(t *testing.T) {
	tt := []struct {
		Name     string
		Closed   bool
		Setup    func(h *mockAMQPConnectionHandlers)
		Expected func(t *testing.T, ch amqp.Channel, err error)
	}{
		{
			Name: "Valid",
			Setup: func(h *mockAMQPConnectionHandlers) {
				h.Channel = func() (*amqp091.Channel, error) {
					return &amqp091.Channel{}, nil
				}
			},
			Expected: func(t *testing.T, ch amqp.Channel, err error) {
				assert.IsType(t, &channel{}, ch)
				assert.NoError(t, err)
			},
		},
		{
			Name: "ErrFromAMQP",
			Setup: func(h *mockAMQPConnectionHandlers) {
				h.Channel = func() (*amqp091.Channel, error) {
					return nil, errors.New("could not create channel")
				}
			},
			Expected: func(t *testing.T, ch amqp.Channel, err error) {
				assert.Nil(t, ch)
				assert.Error(t, err)
			},
		},
		{
			Name: "AlreadyClosed",
			Setup: func(h *mockAMQPConnectionHandlers) {
				h.Channel = func() (*amqp091.Channel, error) {
					panic("should not be called")
				}
				h.IsClosed = func() bool {
					return true // already closed
				}
			},
			Closed: true,
			Expected: func(t *testing.T, ch amqp.Channel, err error) {
				assert.Nil(t, ch)
				assert.Error(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			h := newDefaultAMQPConnectionHandlers()
			tc.Setup(&h)
			c := &connection{
				closed:     tc.Closed,
				Connection: &mockAMQPConnection{h: h},
				ctx:        context.Background(),
			}
			ch, err := c.Channel()
			tc.Expected(t, ch, err)
		})
	}
}

func TestConnection_Close(t *testing.T) {
	tt := []struct {
		Name     string
		Setup    func(h *mockAMQPConnectionHandlers)
		Expected func(t *testing.T, err error)
	}{
		{
			Name: "Valid",
			Setup: func(h *mockAMQPConnectionHandlers) {
				h.Close = func() error {
					return nil
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			Name: "ErrFromAMQP",
			Setup: func(h *mockAMQPConnectionHandlers) {
				h.Close = func() error {
					return errors.New("could not create channel")
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
		{
			Name: "AlreadyClosed",
			Setup: func(h *mockAMQPConnectionHandlers) {
				h.Close = func() error {
					panic("should not be called")
				}
				h.IsClosed = func() bool {
					return true // already closed.
				}
			},
			Expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			h := newDefaultAMQPConnectionHandlers()
			tc.Setup(&h)
			c := &connection{
				Connection: &mockAMQPConnection{h: h},
				ctx:        context.Background(),
			}
			err := c.Close()
			tc.Expected(t, err)
		})
	}
}

// closedAMQPConnection generates a connection where
// the underlined AMQP connection is deemed closed.
func closedAMQPConnection() *connection {
	return connectionWithClosedState(true)
}

// openAMQPConnection generates a connection where
// the underlined AMQP connection is deemed open.
func openAMQPConnection() *connection {
	return connectionWithClosedState(false)
}

// connectionWithClosedState allows us to toggle the closed state on
// an underlined AMQP connection.
func connectionWithClosedState(state bool) *connection {
	h := newDefaultAMQPConnectionHandlers()
	h.IsClosed = func() bool {
		return state
	}
	return &connection{Connection: &mockAMQPConnection{h: h}}
}

func TestConnection_IsClosed(t *testing.T) {
	tt := []struct {
		Name     string
		Conn     *connection
		Expected func(t *testing.T, c bool)
	}{
		{"TrueOnNilConnection", nil, func(t *testing.T, c bool) {
			assert.True(t, c)
		}},
		{"TrueOnClosedAMQPConnection", closedAMQPConnection(), func(t *testing.T, c bool) {
			assert.True(t, c)
		}},
		{"FalseOnOpenAMQPConnection", openAMQPConnection(), func(t *testing.T, c bool) {
			assert.False(t, c)
		}},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			tc.Expected(t, tc.Conn.IsClosed())
		})
	}
}

func TestConnection_Reconnection(t *testing.T) {
	closed, cancel := context.WithCancel(context.Background())
	cancel()

	tt := []struct {
		Name     string
		Context  context.Context
		Closed   bool
		Notify   func(n chan *amqp091.Error)
		Expected func(t *testing.T, triggered int64)
	}{
		{
			Name: "NormalConnectionClose",
			Notify: func(n chan *amqp091.Error) {
				close(n)
			},
			Expected: func(t *testing.T, triggered int64) {
				assert.Zero(t, triggered)
			},
		},
		{
			Name: "NormalConnectionCloseWithNil",
			Notify: func(n chan *amqp091.Error) {
				n <- nil
				close(n)
			},
			Expected: func(t *testing.T, triggered int64) {
				assert.Zero(t, triggered)
			},
		},
		{
			Name:    "ClosedContext",
			Context: closed,
			Notify:  func(n chan *amqp091.Error) {},
			Expected: func(t *testing.T, triggered int64) {
				assert.Zero(t, triggered)
			},
		},
		{
			Name: "NonRecoverableError",
			Notify: func(n chan *amqp091.Error) {
				n <- &amqp091.Error{Recover: false, Reason: "its all blown up"}
				close(n)
			},
			Closed: true,
			Expected: func(t *testing.T, triggered int64) {
				assert.Equal(t, int64(1), triggered)
			},
		},
		{
			Name: "RecoverableError",
			Notify: func(n chan *amqp091.Error) {
				n <- &amqp091.Error{Recover: true, Reason: "just a temporary setback"}
				close(n)
			},
			Closed: true,
			Expected: func(t *testing.T, triggered int64) {
				assert.Equal(t, int64(1), triggered)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			h := newDefaultAMQPConnectionHandlers()
			triggered := int64(0)

			var wg sync.WaitGroup
			wg.Add(1)

			// we also wrap the close function to handle
			// situations where a normal shutdown was triggered
			h.Close = func() error {
				if atomic.LoadInt64(&triggered) > 0 {
					return nil // we have attempted to reconnect.
				}

				defer wg.Done()
				return nil
			}

			ctx := context.Background()
			if tc.Context != nil {
				ctx = tc.Context
			}

			h.NotifyClose = func(ch chan *amqp091.Error) chan *amqp091.Error {
				go func() {
					tc.Notify(ch)
				}()
				return ch
			}

			h.IsClosed = func() bool {
				return tc.Closed
			}

			conn := &mockAMQPConnection{h: h}

			dialer := func() (amqp091Connection, error) {
				if atomic.LoadInt64(&triggered) > 0 {
					return conn, nil // we have attempted to reconnect.
				}
				defer wg.Done()
				atomic.AddInt64(&triggered, 1)
				return conn, nil
			}

			c := &connection{
				dialer:     dialer,
				ctx:        ctx,
				Connection: conn,
			}

			go c.background()
			wg.Wait()

			tc.Expected(t, atomic.LoadInt64(&triggered))
		})
	}
}
