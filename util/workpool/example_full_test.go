package workpool

import (
"fmt"
)

// gen creates a closed channel with the nums arguments.
func gen(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		for _, n := range nums {
			out <- n
		}
		close(out)
	}()
	return out
}

// sq connects an input channel to an output channel with a squaring function. If it detects the channel is closed false
// is returned, otherwise it processes one number and returns.
func sq(input <-chan int, output chan<- int) WorkHandler {
	return func(done <-chan struct{}) bool {
		for number := range input {
			output <- number * number
			return true
		}
		return false
	}
}

func ExampleWorkPool() {
	// Closed input channel with three values.
	var input <-chan int = gen(2, 3, 10)

	// Output channel for results.
	output := make(chan int)

	// Close function called when pool exits.
	closer := func() {
		close(output)
	}

	// Create a pool using a squaring function WorkHandler and a single worker.
	pool := NewWithClose(1, sq(input, output), closer)
	go pool.Run()

	// Check results
	for num := range output {
		fmt.Println(num) // 4 9 100
	}
}
