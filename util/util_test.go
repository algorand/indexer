package util

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCallWithTimeout_timeout(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		<-done
	}()

	err := CallWithTimeout(context.Background(), 1*time.Nanosecond, func(ctx context.Context) error {
		defer close(done)
		time.Sleep(10 * time.Nanosecond)
		return errors.New("should not return")
	})

	require.Error(t, err, "There should be an error")
	require.ErrorIs(t, err, ErrTimeout)
}

func TestCallWithTimeout_noTimeout(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		<-done
	}()

	CallError := errors.New("this should be the result")
	err := CallWithTimeout(context.Background(), 1*time.Minute, func(ctx context.Context) error {
		defer close(done)
		return CallError
	})

	require.Error(t, err, "There should be an error")
	require.ErrorIs(t, err, CallError)
}
