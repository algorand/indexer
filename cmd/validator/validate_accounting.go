package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/algorand/go-algorand-sdk/types"
)

// Params are the program arguments which need to be passed between objects.
type Params struct {
	algodURL     string
	algodToken   string
	indexerURL   string
	indexerToken string
	retries      int
}

func init() {
	errorLog = log.New(os.Stderr, "", 1)
	errorLog.SetFlags(0)
}

var errorLog *log.Logger

// Processor is the algorithm to fetch and compare data from indexer and algod
type Processor interface {
	ProcessAddress(addr string, config Params) (Result, error)
}

// Result is the output of ProcessAddress.
type Result struct {
	Equal   bool
	Error   error
	Retries int
	Details *ErrorDetails
}

// ErrorDetails are additional details attached to a result in the event of an error.
type ErrorDetails struct {
	address string
	algod   string
	indexer string
	diff    []string
}

func main() {
	var (
		config       Params
		addr         string
		threads      int
		processorNum int
	)

	flag.StringVar(&config.algodURL, "algod-url", "", "Algod url.")
	flag.StringVar(&config.algodToken, "algod-token", "", "Algod token.")
	flag.StringVar(&config.indexerURL, "indexer-url", "", "Indexer url.")
	flag.StringVar(&config.indexerToken, "indexer-token", "", "Indexer toke.n")
	flag.StringVar(&addr, "addr", "", "If provided validate a single address instead of reading Stdin.")
	flag.IntVar(&config.retries, "retries", 0, "Number of retry attempts when a difference is detected.")
	flag.IntVar(&threads, "threads", 10, "Number of worker threads to initialize.")
	flag.IntVar(&processorNum, "processor", 0, "Choose compare algorithm [0 = Struct, 1 = Reflection]")
	flag.Parse()

	if len(config.algodURL) == 0 {
		errorLog.Fatalf("algod-url parameter is required.")
	}
	if len(config.algodToken) == 0 {
		errorLog.Fatalf("algod-token parameter is required.")
	}
	if len(config.indexerURL) == 0 {
		errorLog.Fatalf("indexer-url parameter is required.")
	}

	var processor Processor
	switch processorNum {
	case 0:
		processor = StructProcessor{}
	case 1:
		processor = DynamicProcessor{}
	default:
		errorLog.Fatalf("invalid processor selected.")
	}

	results := make(chan Result, 5000)

	// Process a single address
	if len(addr) != 0 {
		callProcessor(processor, addr, config, results)
		close(results)
	} else {
		go start(processor, threads, config, results)
	}

	// This will keep going until the results channel is closed.
	resultsPrinter(config, results)
}

// start kicks off a bunch of  go routines to compare addresses, it also creates a work channel to feed the workers and
// fills the work channel by reading from os.Stdin. Results are returned to the results channel.
func start(processor Processor, threads int, config Params, results chan<- Result) {
	var wg sync.WaitGroup
	work := make(chan string, 100 * threads)

	// Start the workers
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range work {
				callProcessor(processor, addr, config, results)
			}
		}()
	}

	// Read addresses from stdin and pass along to workers
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			work <- scanner.Text()
		}
		close(work)
	}()

	// Wait for workers to finish then close the results.
	wg.Wait()
	close(results)
}

// normalizeAddress accepts an algorand address or base64 encoded address and outputs the algorand address
func normalizeAddress(addr string) (string, error) {
	_, err := types.DecodeAddress(addr)
	if err == nil {
		return addr, nil
	}

	addrBytes, err := base64.StdEncoding.DecodeString(addr)
	if err  == nil {
		var address types.Address
		copy(address[:], addrBytes)
		return address.String(), nil
	}

	return "", errors.New("unable to decode address")
}

// callProcessor invokes the processor with a retry mechanism.
func callProcessor(processor Processor, addrInput string, config Params, results chan<- Result) {
	addr, err := normalizeAddress(addrInput)
	if err != nil {
		results <- Result{
			Equal:   false,
			Error:   err,
			Retries: 0,
			Details: &ErrorDetails{
				address: addrInput,
			},
		}
		return
	}

	for i := 0; true; i++ {
		result, err := processor.ProcessAddress(addr, config)
		if err == nil && (result.Equal || i >= config.retries){
			// Return when results are equal, or when finished retrying.
			result.Retries = i
			results <- result
			return
		} else if err != nil {
			// If there is an error return immediately and cram the error.
			results <- Result{
				Equal:   false,
				Error:   err,
				Retries: i,
				Details: &ErrorDetails{
					address: addr,
				},
			}
			return
		}

		// Wait before trying again in case there is an indexer/algod synchronization issue.
		time.Sleep(1 * time.Second)
	}
}

// resultRune picks the appropriate rune for the status output.
func resultRune(success bool, retries int) rune {
	if success && retries == 0{
		return '.'
	}
	if success {
		return rune(retries)
	}
	return 'X'
}

// resultsPrinter reads the results channel and prints it to the error log.
func resultsPrinter(config Params, results <-chan Result) {
	numResults := 0
	numErrors := 0
	numRetries := 0
	startTime := time.Now()

	stats := func() {
		endTime := time.Now()
		fmt.Printf("\n\nNumber of errors: [%d / %d]\n", numErrors, numResults)
		fmt.Printf("Retry count: %d\n", numRetries)
		fmt.Printf("Test duration: %s\n", time.Time{}.Add(endTime.Sub(startTime)).Format("15:04:05"))
	}

	// Print stats at the end when things terminate naturally.
	defer stats()

	// Also print stats as the program exists after being interrupted.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<- quit
		stats()
		os.Exit(1)
	}()

	// Process results. Print progress to stdout and log errors to errorLog.
	for r := range results {
		if numResults % 100 == 0 {
			fmt.Printf("\n%-8d : ", numResults)
		}
		fmt.Printf("%c", resultRune(r.Equal, r.Retries))

		numResults++
		numRetries+=r.Retries
		if r.Error != nil || !r.Equal {
			errorLog.Printf("===================================================================")
			errorLog.Printf("%s", time.Now().Format("2006-01-02 3:4:5 PM"))
			errorLog.Printf("Account: %s", r.Details.address)
			errorLog.Printf("Error #: %d", numErrors)

			// Print error message if there is one.
			if r.Error != nil {
				errorLog.Printf("Processor error: %v\n", r.Error)
			}
			// Print error details if there are any.
			if r.Details != nil {
				numErrors++
				errorLog.Printf("Algod Details:\n%s", r.Details.algod)
				errorLog.Printf("Indexer Details:\n%s", r.Details.indexer)
				errorLog.Printf("Differences:")
				for _, diff := range r.Details.diff {
					errorLog.Printf("     - %s", diff)
				}
				errorLog.Printf("echo 'Algod:'")
				errorLog.Printf("curl -q -s -H 'Authorization: Bearer %s' '%s/v2/accounts/%s?pretty'", config.algodToken, config.algodURL, r.Details.address)
				errorLog.Printf("echo 'Indexer:'")
				errorLog.Printf("curl -q -s -H 'Authorization: Bearer %s' '%s/v2/accounts/%s?pretty'", config.indexerToken, config.indexerURL, r.Details.address)
			}
		}
	}
}