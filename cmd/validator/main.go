package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/algorand/indexer/cmd/validator/core"
)

func main() {
	var (
		config       core.Params
		addr         string
		threads      int
		processorNum int
		printCurl    bool
	)

	flag.StringVar(&config.AlgodURL, "algod-url", "", "Algod url.")
	flag.StringVar(&config.AlgodToken, "algod-token", "", "Algod token.")
	flag.StringVar(&config.IndexerURL, "indexer-url", "", "Indexer url.")
	flag.StringVar(&config.IndexerToken, "indexer-token", "", "Indexer toke.n")
	flag.StringVar(&addr, "addr", "", "If provided validate a single address instead of reading Stdin.")
	flag.IntVar(&config.Retries, "retries", 5, "Number of retry attempts when a difference is detected.")
	flag.IntVar(&config.RetryDelayMS, "retry-delay", 1000, "Time in milliseconds to sleep between retries.")
	flag.IntVar(&threads, "threads", 4, "Number of worker threads to initialize.")
	flag.IntVar(&processorNum, "processor", 0, "Choose compare algorithm [0 = Struct, 1 = Reflection]")
	flag.BoolVar(&printCurl, "print-commands", false, "Print curl commands, including tokens, to query algod and indexer.")
	flag.Parse()

	if len(config.AlgodURL) == 0 {
		core.ErrorLog.Fatalf("algod-url parameter is required.")
	}
	if len(config.AlgodToken) == 0 {
		core.ErrorLog.Fatalf("algod-token parameter is required.")
	}
	if len(config.IndexerURL) == 0 {
		core.ErrorLog.Fatalf("indexer-url parameter is required.")
	}

	results := make(chan core.Result, 10)

	go func() {
		if len(addr) != 0 {
			processor, err := core.MakeProcessor(core.ProcessorID(processorNum))
			if err != nil {
				core.ErrorLog.Fatalf("%s.\n", err)
			}

			// Process a single address
			core.CallProcessor(processor, addr, config, results)
			close(results)
		} else {
			// Process from stdin
			start(core.ProcessorID(processorNum), threads, config, results)
		}
	}()

	// This will keep going until the results channel is closed.
	numErrors := resultsPrinter(config, printCurl, results)
	if numErrors > 0 {
		os.Exit(1)
	}
}

// start kicks off a bunch of  go routines to compare addresses, it also creates a work channel to feed the workers and
// fills the work channel by reading from os.Stdin. Results are returned to the results channel.
func start(processorID core.ProcessorID, threads int, config core.Params, results chan<- core.Result) {
	work := make(chan string, 100*threads)

	// Read addresses from stdin and pass along to workers
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			work <- scanner.Text()
		}
		close(work)
	}()

	core.Start(work, processorID, threads, config, results)
}

// resultChar picks the appropriate status character for the output.
func resultChar(success bool, retries int) string {
	if success && retries == 0 {
		return "."
	}
	if success && retries > 9 {
		return fmt.Sprintf("(%d)", retries)
	}
	if success {
		return fmt.Sprintf("%d", retries)
	}
	return "X"
}

// resultsPrinter reads the results channel and prints it to the error log. Returns the number of errors.
func resultsPrinter(config core.Params, printCurl bool, results <-chan core.Result) int {
	numResults := 0
	numErrors := 0
	numRetries := 0
	startTime := time.Now()

	stats := func() {
		endTime := time.Now()
		duration := endTime.Sub(startTime)
		fmt.Printf("\n\nNumber of errors: [%d / %d]\n", numErrors, numResults)
		fmt.Printf("Retry count: %d\n", numRetries)
		fmt.Printf("Checks per second: %f\n", float64(numResults+numRetries)/duration.Seconds())
		fmt.Printf("Test duration: %s\n", time.Time{}.Add(duration).Format("15:04:05"))
	}

	// Print stats at the end when things terminate naturally.
	defer stats()

	// Also print stats as the program exits after being interrupted.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		stats()
		os.Exit(1)
	}()

	// Process results. Print progress to stdout and log errors to errorLog.
	for r := range results {
		if numResults%100 == 0 {
			fmt.Printf("\n%-8d : ", numResults)
		}
		fmt.Printf("%s", resultChar(r.Equal, r.Retries))

		numResults++
		numRetries += r.Retries
		if r.Error != nil || !r.Equal {
			numErrors++
			core.ErrorLog.Printf("===================================================================")
			core.ErrorLog.Printf("%s", time.Now().Format("2006-01-02 3:4:5 PM"))
			core.ErrorLog.Printf("Account: %s", r.Details.Address)
			core.ErrorLog.Printf("Error #: %d", numErrors)
			core.ErrorLog.Printf("Retries: %d", r.Retries)
			core.ErrorLog.Printf("Rounds Match: %t", r.SameRound)

			// Print error message if there is one.
			if r.Error != nil {
				core.ErrorLog.Printf("Processor error: %v\n", r.Error)
			} else {
				// Print error details if there are any.
				if r.Details != nil {
					core.ErrorLog.Printf("Algod Details:\n%s", r.Details.Algod)
					core.ErrorLog.Printf("Indexer Details:\n%s", r.Details.Indexer)
					core.ErrorLog.Printf("Differences:")
					for _, diff := range r.Details.Diff {
						core.ErrorLog.Printf("     - %s", diff)
					}
				}
				// Optionally print curl command.
				if printCurl {
					core.ErrorLog.Printf("echo 'Algod:'")
					core.ErrorLog.Printf("curl -q -s -H 'Authorization: Bearer %s' '%s/v2/accounts/%s?pretty'", config.AlgodToken, config.AlgodURL, r.Details.Address)
					core.ErrorLog.Printf("echo 'Indexer:'")
					core.ErrorLog.Printf("curl -q -s -H 'Authorization: Bearer %s' '%s/v2/accounts/%s?pretty'", config.IndexerToken, config.IndexerURL, r.Details.Address)
				}
			}
		}
	}

	return numErrors
}
