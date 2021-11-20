package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/go-codec/codec"

	log "github.com/sirupsen/logrus"
)

// ErrTimeout is returned when CallWithTimeout has a normal timeout.
var ErrTimeout = errors.New("timeout during call")

// IsTimeoutError compares the given error against the timeout errors.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrTimeout)
}

// errMisbehavingHandler is written to the log when a handler does not return.
var errMisbehavingHandler = "Misbehaving handler did not exist after 1 second."

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

// CallWithTimeout manages the channel / select loop required for timing
// out a function using a WithTimeout context. No timeout if timeout = 0.
// A new context is passed into handler, and cancelled at the end of this
// call.
func CallWithTimeout(ctx context.Context, log *log.Logger, timeout time.Duration, handler func(ctx context.Context) error) error {
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
			return ErrTimeout
		}
		return err
	case <-timeoutCtx.Done():
		go misbehavingHandlerDetector(log, done)
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return ErrTimeout
		}
		return timeoutCtx.Err()
	}
}

// PrintableUTF8OrEmpty checks to see if the entire string is a UTF8 printable string.
// If this is the case, the string is returned as is. Otherwise, the empty string is returned.
func PrintableUTF8OrEmpty(in string) string {
	// iterate throughout all the characters in the string to see if they are all printable.
	// when range iterating on go strings, go decode each element as a utf8 rune.
	for _, c := range in {
		// is this a printable character, or invalid rune ?
		if c == utf8.RuneError || !unicode.IsPrint(c) {
			return ""
		}
	}
	return in
}

// KeysStringBool returns all of the keys in the map joined by a comma.
func KeysStringBool(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// MaybeFail exits if there was an error.
func MaybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
	os.Exit(1)
}

var oneLineJSONCodecHandle *codec.JsonHandle

// JSONOneLine converts an object into JSON
func JSONOneLine(obj interface{}) string {
	var b []byte
	enc := codec.NewEncoderBytes(&b, oneLineJSONCodecHandle)
	enc.MustEncode(obj)
	return string(b)
}

func init() {
	oneLineJSONCodecHandle = new(codec.JsonHandle)
	oneLineJSONCodecHandle.ErrorIfNoField = true
	oneLineJSONCodecHandle.ErrorIfNoArrayExpand = true
	oneLineJSONCodecHandle.Canonical = true
	oneLineJSONCodecHandle.RecursiveEmptyCheck = true
	oneLineJSONCodecHandle.HTMLCharsAsIs = true
	oneLineJSONCodecHandle.Indent = 0
	oneLineJSONCodecHandle.MapKeyAsString = true
}
