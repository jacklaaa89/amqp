// +build integration

package rabbitmq

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	rh "github.com/michaelklishin/rabbit-hole/v2"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

// forceConnectionClose this will close ALL connections
// on the connected rabbit-mq instance.
//
// not intended for use on production instances.
func forceConnectionClose(t *testing.T) {
	m, err := rh.NewClient(managementURLFromEnv(), "guest", "guest")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// keep going until at least a single connection is closed or a timeout happens
forever:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout attempting to force connection close")
		default:
			conns, lErr := m.ListConnections()
			require.NoError(t, lErr)

			for _, conn := range conns {
				if conn.Vhost != vHost {
					continue
				}
				_, rErr := m.CloseConnection(conn.Name)
				require.NoError(t, rErr)
				break forever
			}
		}
	}
}

func TestChannel_Reconnection_Integration(t *testing.T) {
	setup(t)
	defer teardown(t)

	dialer := DialConfig(context.Background(), integrationURLFromEnv(), Config{
		SASL: []amqp091.Authentication{
			&PlainAuth{
				Username: "guest",
				Password: "guest",
			},
		},
		Vhost: vHost,
	})

	// create a new connection.
	conn, err := dialer()
	require.NoError(t, err)

	// create new channel.
	ch, err := conn.Channel()
	require.NoError(t, err)

	// listen to reconnects on the channel.
	var reconnected int64
	ch.NotifyReconnect(func() {
		atomic.StoreInt64(&reconnected, 1)
	})

	// create a new queue for testing.
	q, err := ch.CreateQueue(context.Background(), "test-queue", true, false, false)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	// start two go-routines which continuously publish and consume from a queue
	// the publish will continuously send the reconnection state.
	go func() {
		// publish the reconnected int to the queue
		for {
			time.Sleep(time.Second) // add some jitter.

			buf := &bytes.Buffer{}
			require.NoError(t, json.NewEncoder(buf).Encode(atomic.LoadInt64(&reconnected)))
			_ = ch.Publish(context.Background(), "", "test-queue", buf)
		}
	}()

	// and the consume will stop consuming once we have received that we have reconnected once
	// this proves that:
	// a: the `d` chan did not close mid reconnect
	// b: we are still able to consume after a reconnection has occurred
	// c: the notification omitter for reconnections has worked to tell use we have reconnected
	go func() {
		defer wg.Done()
		d, cancel, cErr := q.Consume(context.Background(), "test-consumer", true, false)
		require.NoError(t, cErr)
		defer cancel()

		for msg := range d {
			var body int64
			require.NoError(t, json.NewDecoder(msg.Body()).Decode(&body))
			if body > 0 {
				break
			}
		}
	}()

	// allow normal operation for a small amount of time.
	time.Sleep(2 * time.Second)

	// use the management API to force a connection close which is
	// outside of the control of this library.
	forceConnectionClose(t)

	// wait for the the consume to receive the message that we
	// reconnected.
	wg.Wait()

	// assert one reconnection occurred.
	assert.Equal(t, int64(1), atomic.LoadInt64(&reconnected))

	// clean-up connection.
	conn.Close()
}
