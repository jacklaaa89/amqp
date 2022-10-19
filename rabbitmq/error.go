package rabbitmq

import (
	"github.com/rabbitmq/amqp091-go"

	"github.com/jacklaaa89/amqp"
)

// notifier helper interface which wraps notification methods
// which are usually shared by different types.
type notifier interface {
	// NotifyClose the internal amqp091 function defined on both
	// channels and connections which set up notifications for errors.
	NotifyClose(rcv chan *amqp091.Error) chan *amqp091.Error
}

// amqpError represents a wrapped amqp091.Error
type amqpError struct {
	*amqp091.Error
}

// Code returns the AMQP error code.
func (a *amqpError) Code() int {
	return a.Error.Code
}

// Reason returns the error description
func (a *amqpError) Reason() string {
	return a.Error.Reason
}

// Recover whether the error is recoverable.
func (a *amqpError) Recover() bool {
	return a.Error.Recover
}

// FromServer whether the close originated from the client or server.
func (a *amqpError) FromServer() bool {
	return a.Error.Server
}

// handleNotifyError helper function to handle notification of errors.
func handleNotifyError(ch notifier, fn amqp.ErrorNotificationFunc) {
	rcv := make(chan *amqp091.Error)
	ch.NotifyClose(rcv)

	go func() {
		for e := range rcv {
			fn(&amqpError{e})
		}
	}()
}
