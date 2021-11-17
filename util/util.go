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
)

// ErrTimeout is returned when CallWithTimeout has a normal timeout.
var ErrTimeout = errors.New("timeout during call")

// ErrUnknownTimeoutExit is returned when CallWithTimeout has an unexpected done event.
var ErrUnknownTimeoutExit = errors.New("unexpected exit during timeout")

// CallWithTimeout manages the channel / select loop required for timing
// out a function using a WithTimeout context.
func CallWithTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	done := make(chan error)

	// Call the long function
	var err error
	go func(routineCtx context.Context) {
		err = fn(routineCtx)
		close(done)
	}(ctx)

	select { // wait for task to finish or for a timeout.
	case <-time.After(timeout):
		return ErrTimeout
	case _, ok := <-done:
		if !ok {
			// channel was closed as expected, use err object.
			return err
		}
		return ErrUnknownTimeoutExit
	}

	// WithTimeout context isn't working properly.
	// It should be used so that the DB operation will be cancelled.
	/*
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Call the long function
		done := make(chan error)
		var err error
		go func(routineCtx context.Context) {
			err = fn(routineCtx)
			close(done)
		}(timeoutCtx)

		select { // wait for task to finish or context to timeout/cancel
		case <-timeoutCtx.Done()
			if timeoutCtx.Err() == context.DeadlineExceeded {
			    return ErrTimeout
			}
			return timeoutCtx.Err()
		case _, ok := <-done:
			if !ok {
				// channel was closed as expected, use err object.
				return err
			}
			return ErrUnknownTimeoutExit
		}
	*/
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
