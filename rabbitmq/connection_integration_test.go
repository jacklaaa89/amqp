// +build integration

package rabbitmq

import (
	"context"
	"os"
	"testing"

	rh "github.com/michaelklishin/rabbit-hole/v2"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vHost the virtual hosts to run tests on.
const vHost = "/integration"

func integrationURLFromEnv() string {
	url := os.Getenv("AMQP_URL")
	if url == "" {
		url = "amqp://"
	}
	return url
}

func managementURLFromEnv() string {
	url := os.Getenv("AMQP_MANAGEMENT_URL")
	if url == "" {
		url = "http://localhost:15672"
	}
	return url
}

// teardown will connect to the locally running cluster
// and the integration v-host, this will remove any state
// set by previous tests.
func teardown(t *testing.T) {
	mAPI, err := rh.NewClient(managementURLFromEnv(), "guest", "guest")
	require.NoError(t, err)

	// delete vhost
	_, err = mAPI.DeleteVhost(vHost)
	require.NoError(t, err)
}

// setup performs an initial teardown and then creates the v-host used for testing.
func setup(t *testing.T) {
	teardown(t)

	mAPI, err := rh.NewClient(managementURLFromEnv(), "guest", "guest")
	require.NoError(t, err)

	// ensure vhost
	_, err = mAPI.PutVhost(vHost, rh.VhostSettings{
		Description: "virtual host used for integration testing",
	})
	require.NoError(t, err)
}

func reset() {
	dial = amqp091.Dial
	dialConfig = amqp091.DialConfig
}

// TestDial_Integration tests we can successfully connect to a locally running rabbitmq-server
func TestDial_Integration(t *testing.T) {
	reset()
	dialer := Dial(context.Background(), integrationURLFromEnv())
	con, err := dialer()
	assert.NoError(t, err)
	assert.NotNil(t, con)
	assert.NoError(t, con.Close())
}

// TestDial_Integration tests we can successfully connect to a locally running rabbitmq-server
func TestDialConfig_Integration(t *testing.T) {
	reset()
	dialer := DialConfig(context.Background(), integrationURLFromEnv(), Config{
		SASL: []Authentication{
			&PlainAuth{"guest", "guest"},
		},
	})
	con, err := dialer()
	assert.NoError(t, err)
	assert.NotNil(t, con)
	assert.NoError(t, con.Close())
}
