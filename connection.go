package amqp

import (
	"io"
)

// Dialer represents a function which returns a connection and an error.
type Dialer func() (Connection, error)

// Error represents an error from AMQP.
type Error interface {
	// Code returns the constant code from the specification
	Code() int
	// Reason returns the description of the error
	Reason() string
	// Recover returns true when this error can be recovered by retrying later or with different parameters
	Recover() bool
	// FromServer returns true when initiated from the server, false when from this library
	FromServer() bool
}

// Notifier an interface for types which omit events.
type Notifier interface {
	// NotifyClose triggers the supplied function when a close happens
	// this will be a graceful close only, i.e. triggered from the SDK
	// on a connection this will be triggered on both the connection and all the
	// channels
	// on a channel it will only trigger on the channel.
	NotifyClose(fn func())
	// NotifyReconnect triggers the supplied function when a reconnection
	// is successful.
	// on a connection this will be triggered on both the connection and all the
	// channels
	// on a channel it will only trigger on the channel.
	NotifyReconnect(fn func())
}

// Connection represents a message amqp compatible broker connection
// each broker implementation which handle (usually statically) which
// amqp protocol they implement.
//
// The AMQP spec is not entirely implemented in this context, only the functions which
// I require from the broker.
type Connection interface {
	io.Closer
	Notifier

	// Channel attempts to create a new channel to perform actions against
	// typically it is one global connection, and then one channel per thread.
	Channel() (Channel, error)
	// IsClosed determines if the connection is closed.
	IsClosed() bool
}
