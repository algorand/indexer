package api

import (
	"context"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
)

// ErrTimeout is returned when callWithTimeout has a normal timeout.
var errTimeout = errors.New("timeout during call")

// isTimeoutError compares the given error against the timeout errors.
func isTimeoutError(err error) bool {
	return errors.Is(err, errTimeout)
}

// errMisbehavingHandler is written to the log when a handler does not return.
var errMisbehavingHandler = "Misbehaving handler did not exit after 1 second."

// misbehavingHandlerDetector warn if ch does not exit after 1 second.
func misbehavingHandlerDetector(log *log.Logger, ch chan struct{}) {
	if log == nil {
		return
	}

	select {
	case <-ch:
		// Good. This means the handler returns shortly after the context finished.
		return
	case <-time.After(1 * time.Second):
		log.Warnf(errMisbehavingHandler)
	}
}

// callWithTimeout manages the channel / select loop required for timing
// out a function using a WithTimeout context. No timeout if timeout = 0.
// A new context is passed into handler, and cancelled at the end of this
// call.
func callWithTimeout(ctx context.Context, log *log.Logger, timeout time.Duration, handler func(ctx context.Context) error) error {
	if timeout == 0 {
		return handler(ctx)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Call function in go routine
	done := make(chan struct{})
	var err error
	go func(routineCtx context.Context) {
		err = handler(routineCtx)
		close(done)
	}(timeoutCtx)

	// wait for task to finish or context to timeout/cancel
	select {
	case <-done:
		// This may not be possible, but in theory the handler would quickly terminate
		// when the context deadline is reached. So make sure the handler didn't finish
		// due to a timeout.
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return errTimeout
		}
		return err
	case <-timeoutCtx.Done():
		go misbehavingHandlerDetector(log, done)
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return errTimeout
		}
		return timeoutCtx.Err()
	}
}
