package workpool

func ExampleWorkPool_struct() {
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
	go pool.Run()
}

func ExampleNew() {
	numWorkers := 5
	outputs := make(chan int, numWorkers)

	worker := func(done <-chan struct{}) bool {
		outputs <- 1
		return false
	}

	pool := New(numWorkers, worker)
	go pool.Run()
}

func ExampleNewWithClose() {
	numWorkers := 5
	outputs := make(chan int, numWorkers)

	worker := func(done <-chan struct{}) bool {
		outputs <- 1
		return false
	}
	closer := func() {
		close(outputs)
	}

	pool := NewWithClose(numWorkers, worker, closer)
	go pool.Run()
}