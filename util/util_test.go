package util

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

func TestCallWithTimeout_timeout(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		close(done)
	}()

	logger, hook := test.NewNullLogger()
	err := CallWithTimeout(context.Background(), logger, 1*time.Nanosecond, func(ctx context.Context) error {
		<-done
		return errors.New("should not return")
	})

	require.Error(t, err, "There should be an error")
	require.ErrorIs(t, err, ErrTimeout)

	time.Sleep(2 * time.Second)
	require.Len(t, hook.Entries, 1)
	require.Equal(t, ErrMisbehavingHandler, hook.LastEntry().Message)
}

func TestCallWithTimeout_noTimeout(t *testing.T) {
	done := make(chan struct{})
	defer func() {
		<-done
	}()

	CallError := errors.New("this should be the result")
	err := CallWithTimeout(context.Background(), nil, 1*time.Minute, func(ctx context.Context) error {
		defer close(done)
		return CallError
	})

	require.Error(t, err, "There should be an error")
	require.ErrorIs(t, err, CallError)
}
