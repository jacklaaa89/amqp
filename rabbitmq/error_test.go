package rabbitmq

import (
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestAmqpError_Code(t *testing.T) {
	assert.Equal(t, 1, (&amqpError{&amqp091.Error{Code: 1}}).Code())
}

func TestAmqpError_FromServer(t *testing.T) {
	assert.True(t, (&amqpError{&amqp091.Error{Server: true}}).FromServer())
}

func TestAmqpError_Reason(t *testing.T) {
	assert.Equal(t, "unexpected error", (&amqpError{&amqp091.Error{Reason: "unexpected error"}}).Reason())
}

func TestAmqpError_Recover(t *testing.T) {
	assert.False(t, (&amqpError{&amqp091.Error{Recover: false}}).Recover())
}
