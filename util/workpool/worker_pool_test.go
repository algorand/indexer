package workpool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkerPoolExitWhenNoWork(t *testing.T) {
	numWorkers := 5
	outputs := make(chan int, numWorkers)

	worker := func(done <-chan struct{}) bool {
		outputs <- 1
		return false
	}
	closer := func() {
		close(outputs)
	}
	//pool := NewWithClose(numWorkers, worker, closer)
	pool := &WorkPool{
		Handler: worker,
		Workers: numWorkers,
		Close:   closer,
	}

	start := time.Now()
	pool.Run()
	sum := 0
	for result := range outputs {
		sum += result
	}
	end := time.Now()
	dur := end.Sub(start)

	assert.Equal(t, numWorkers, sum)
	assert.True(t, dur < 100 * time.Millisecond)
}

func TestWorkerPoolWithWorkToDo(t *testing.T) {
	numInputs := 100
	numWorkers := 4
	inputs := make(chan int, numInputs)
	outputs := make(chan int, numInputs)

	worker := func(done <-chan struct{}) bool {
		// This construct is the trickiest part
		for i := range inputs {
			outputs <- i
			return true
		}
		return false
	}
	closer := func() {
		close(outputs)
	}

	pool := WorkPool{
		Handler: worker,
		Workers: numWorkers,
		Close:   closer,
	}

	go func() {
		for i := 0; i < 100; i++ {
			inputs <- i + 1
		}
		close(inputs)
	}()

	start := time.Now()
	pool.Run()
	sum := 0
	for result := range outputs {
		sum += result
	}
	end := time.Now()
	dur := end.Sub(start)

	assert.Equal(t, 100 * (100 + 1) / 2, sum)
	assert.True(t, dur < 100 * time.Millisecond)
}

func TestConcurrency(t *testing.T) {
	tests := []struct{
		inputs    int
		workers   int
		sleep     time.Duration
		intervals int
	}{
		// inputs == workers, expect perfect concurrency for 1 interval
		{
			inputs:    5,
			workers:   5,
			sleep:     100 * time.Millisecond,
			intervals: 1,
		},
		// inputs == workers + 1, expect 2 intervals as one worker processes two inputs
		{
			inputs:  6,
			workers: 5,
			sleep:     100 * time.Millisecond,
			intervals: 2,
		},
		// a single worker processes inputs serially
		{
			inputs:  3,
			workers: 1,
			sleep:     100 * time.Millisecond,
			intervals: 3,
		},
		// inputs == workers * 2, expect perfect concurrency for 2 intervals
		{
			inputs:  6,
			workers: 3,
			sleep:     100 * time.Millisecond,
			intervals: 2,
		},
	}

	for _, test := range tests {
		numWorkers := test.workers
		inputs := make(chan int, test.inputs)
		outputs := make(chan int, test.inputs)

		worker := func(done <-chan struct{}) bool {
			for i := range inputs {
				// Simulate work time
				time.Sleep(test.sleep)
				outputs <- i
				return true
			}
			return false
		}
		closer := func() {
			close(outputs)
			// not interested in outputs for this test
			for len(outputs) > 0 {
				<-outputs
			}
		}

		// Configure pool
		pool := &WorkPool{
			Handler: worker,
			Workers: numWorkers,
			Close:   closer,
		}

		// Initialize input channel.
		for i := 0; i < test.inputs; i++ {
			inputs <- 1
		}
		close(inputs)

		// Process work
		start := time.Now()
		pool.Run()

		assert.WithinDuration(t, start.Add(test.sleep * time.Duration(test.intervals)), time.Now(), 1 * time.Millisecond)
	}
}

// cleanup closes and drains the channel.
func cleanup(ch chan int) {
	close(ch)
	for len(ch) > 0 {
		<-ch
	}
}

// TestCancel ensures that the pool can be cancelled while processing work.
func TestCancel(t *testing.T) {
	numWorkers := 1
	numInputs := 100
	inputs := make(chan int, numInputs)
	outputs := make(chan int, numInputs)

	worker := func(done <-chan struct{}) bool {
		for i := range inputs {
			// Simulate work time
			time.Sleep(100 * time.Microsecond)
			outputs <- i
			return true
		}
		return false
	}
	closer := func() {
		close(outputs)
	}

	// Configure pool
	pool := &WorkPool{
		Handler: worker,
		Workers: numWorkers,
		Close:   closer,
	}

	// Initialize input channel, don't close
	for i := 0; i < numInputs; i++ {
		inputs <- 1
	}

	// Cancel after enough time to process 10 inputs.
	go func() {
		time.Sleep(1 * time.Millisecond)
		pool.Cancel()
	}()
	pool.Run()
	cleanup(inputs)

	processedCount := len(outputs)
	for len(outputs) > 0 {
		<-outputs
	}

	// Should not have been able to process everything
	assert.Less(t, processedCount, numInputs)
}

// TestCancelWithOpenInputChannel ensures that the pool is gracefully stopped while workers are awaiting work.
func TestCancelWithOpenInputChannel(t *testing.T) {
	numWorkers := 1
	started := make(chan struct{})

	worker := func(done <-chan struct{}) bool {
		close(started)
		select {
		case <-time.After(1 * time.Hour):
			return true
		case <-done:
			return false
		}
		return true
	}

	// Configure pool
	pool := &WorkPool{
		Workers: numWorkers,
		Handler: worker,
	}

	// Wait for the worker to start, then cancel it.
	go func() {
		<-started
		pool.Cancel()
	}()

	pool.Run()
}
