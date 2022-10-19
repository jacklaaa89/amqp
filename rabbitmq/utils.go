package rabbitmq

import (
	"context"
	"io"
	"log"

	"github.com/cenkalti/backoff/v4"
)

// closer represents any stream which can be closed
// this is either a channel or the overall connection.
type closer interface {
	io.Closer
	notifier

	IsClosed() bool // IsClosed determines if a channel or connection is closed.
}

// isClosed helper function to check whether a connection or channel is closed.
func isClosed(ch closer) bool {
	return ch == nil || ch.IsClosed()
}

// logError helper function to log an error.
func logError(_ context.Context, err error) {
	if err == nil {
		return
	}

	log.Printf(err.Error())
}

// newBackoff the function to generate the backoff policy
// a variable in order to reduce the backoff in tests.
var newBackoff = defaultBackoff

// defaultBackoff generates a new backoff to use when performing reconnects.
func defaultBackoff(ctx context.Context) backoff.BackOff {
	return backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3), ctx)
}
