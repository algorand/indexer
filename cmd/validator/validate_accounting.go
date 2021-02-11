package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"
)

type Params struct {
	algodUrl     string
	algodToken   string
	indexerUrl   string
	indexerToken string
	retries      int
}

func init() {
	errorLog = log.New(os.Stderr, "", 1)
}

// Processor is the algorithm to fetch and compare data from indexer and algod
type Processor interface {
	ProcessAddress(addr string, config Params, result chan<- Result) error
}

type Result struct {
	Equal   bool
	Retries int
	Details *ErrorDetails
}

type ErrorDetails struct {
	address string
	algod   string
	indexer string
	diff    []string
}

var (
	errorLog     *log.Logger
	config       Params
	addr         string
	threads      int
	processorNum int
)

const (
	STRUCT_PROCESSOR = iota
	GENERIC_PROCESSOR
)

func main() {
	flag.StringVar(&config.algodUrl, "algod-url", "", "Algod url.")
	flag.StringVar(&config.algodToken, "algod-token", "", "Algod token.")
	flag.StringVar(&config.indexerUrl, "indexer-url", "", "Indexer url.")
	flag.StringVar(&config.indexerToken, "indexer-token", "", "Indexer toke.n")
	flag.StringVar(&addr, "addr", "", "If provided validate a single address instead of reading Stdin.")
	flag.IntVar(&config.retries, "retries", 0, "Number of retry attempts when a difference is detected.")
	flag.IntVar(&threads, "threads", 10, "Number of worker threads to initialize.")
	flag.IntVar(&processorNum, "processor", 0, "Choose compare algorithm [0 = Struct, 1 = Reflection]")
	flag.Parse()

	if len(config.algodUrl) == 0 {
		errorLog.Fatalf("algod-url parameter is required.")
	}
	if len(config.algodToken) == 0 {
		errorLog.Fatalf("algod-token parameter is required.")
	}
	if len(config.indexerUrl) == 0 {
		errorLog.Fatalf("indexer-url parameter is required.")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	var processor Processor
	switch processorNum {
	case 0:
		processor = MakeStructProcessor(config)
	case 1:
		processor = GenericProcessor{}
	default:
		errorLog.Fatalf("invalid processor selected.")
	}

	results := make(chan Result, 5000)

	// Process a single address
	if len(addr) != 0 {
		processor.ProcessAddress(addr, config, results)
		close(results)
	} else {
		go startWorkers(processor, results)
	}

	// This will keep going until the results channel is closed.
	printResults(results, quit)
}

func startWorkers(processor Processor, results chan Result) {
	// Otherwise start the threads and read standard input.
	var wg sync.WaitGroup
	work := make(chan string, 1000000)

	// Start the workers
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range work {
				err := processor.ProcessAddress(addr, config, results)
				if err != nil {
					fmt.Println(err)
				}
			}
		}()
	}

	// Read work from stdin and pass along to workers
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			work <- scanner.Text()
		}
		close(work)
	}()

	// Wait for workers to finish in another goroutine.
	wg.Wait()
	close(results)
}

func resultChar(success bool) string {
	if success {
		return "."
	}
	return "X"
}

func printResults(results <-chan Result, quit <-chan os.Signal) {
	numResults := 0
	numErrors := 0
	numRetries := 0
	startTime := time.Now()

	for r := range results {
		if numResults % 100 == 0 {
			fmt.Printf("\n%-8d : ", numResults)
		}
		fmt.Printf(resultChar(r.Equal))

		numResults++
		numRetries+=r.Retries
		if !r.Equal {
			numErrors++
			errorLog.Printf("===================================================================")
			errorLog.Printf("Account: %s", r.Details.address)
			errorLog.Printf("Error #: %d", numErrors)
			errorLog.Printf("Algod Details:\n%s", r.Details.algod)
			errorLog.Printf("Indexer Details:\n%s", r.Details.indexer)
			errorLog.Printf("Differences:")
			for _, diff := range r.Details.diff {
				errorLog.Printf("     - %s", diff)
			}
		}
	}
	endTime := time.Now()

	// TODO: Print this when the quit signal fires
	fmt.Printf("\n\nNumber of errors: [%d / %d]\n", numErrors, numResults)
	fmt.Printf("Retry count: %d\n", numRetries)
	fmt.Printf("Test duration: %s\n", time.Time{}.Add(endTime.Sub(startTime)).Format("15:04:05"))
}