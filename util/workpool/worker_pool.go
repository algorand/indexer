// Package workpool provides a lightweight abstraction around a work function to make it
// easier to create work pools with early termination. This leaves you free to focus on
// the problem being solved and the data pipeline, while the work pool manages concurrency
// of execution.
//
//
package workpool

import (
	"sync"
)

// WorkHandler is a blocking call which manages the retrieval and processing of work. It should either process all work,
// available, or a single piece of work and return. If you return after processing one piece of work pool will keep
// calling the handler.
//
// Return true if the handler should be called again, otherwise return false to indicate work is complete.
//
// The done signal is triggered if the pool has been cancelled. It indicates that work should terminate immediately.
//
// Where work comes from is implementation dependant, for example: a channel, RabbitMQ, dbus, or any other event system.
//
// Here is a WorkHandler which squares a number. Notice that it is wrapped in a function to pass in the input/output
// channels. By returning after each item it allows the WorkPool to deal with early exits.
//   func sq(input <-chan int, output chan<- int) WorkHandler {
//       return func(done <-chan struct{}) bool {
//          for true {
//              select {
//              case number := <- input:
//                  output <- number * number
//                  //return true
//              case <-done:
//                  return false
//              }
//          }
//       }
//   }
//
// Here is another example which ignores the done channel. In this case the WorkPool will manage early termination, but
// will not be able to do so if the input channel is blocked:
//   func sq(input <-chan int, output chan<- int) WorkHandler {
//       return func(done <-chan struct{}) bool {
//           for number := range input {
//               output <- number * number
//               return true
//           }
//           return false
//       }
//   }
type WorkHandler func(done <-chan struct{}) bool

// New creates a worker pool with a given handler function.
func New(numWorkers int, handler WorkHandler) *WorkPool {
	return &WorkPool{
		Handler: handler,
		Workers: numWorkers,
		done:	 make(chan struct{}),
	}
}

// NewWithClose creates a worker pool with a given handler function and a function to call when shutting down.
func NewWithClose(numWorkers int, handler WorkHandler, close func()) *WorkPool {
	return &WorkPool{
		Handler: handler,
		Workers: numWorkers,
		done:	 make(chan struct{}),
		Close:   close,
	}
}

// WorkPool manages running a WorkHandler in some number of goroutines. It also manages a cancel signal to allow for
// early termination.
type WorkPool struct {
	Handler WorkHandler
	Workers int
	done    chan struct{}
	Close   func()
}

// Run starts the configured number of workers and calls WorkHandler until all work has been processed, or the execution
// is cancelled.
func (p *WorkPool) Run() {
	if p.done == nil {
		p.done = make(chan struct{})
	}
	if p.Close != nil {
		defer p.Close()
	}
	var wg sync.WaitGroup
	// Start workers
	wg.Add(p.Workers)
	for i:= 0; i < p.Workers; i++ {
		go func() {
			defer wg.Done()
			handler := p.Handler
			for true {
				select {
				case <-p.done:
					return
				default:
					foundWork := handler(p.done)
					if !foundWork {
						return
					}
				}
			}
		}()
	}

	// Wait until the goroutines finish. By cancellation or otherwise.
	wg.Wait()
}

// Cancel may be called asynchronously to signal that the pool should stop processing work and return to the caller. A
// done signal will be sent to each WorkHandler to allow for graceful shutdown.
func (p *WorkPool) Cancel() {
	close(p.done)
}
