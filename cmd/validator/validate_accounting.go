package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
)

type params struct {
	algodUrl     string
	algodToken   string
	indexerUrl   string
	indexerToken string
	retries      int
}

var (
	errorLog  *log.Logger
	config    params
	addr      string
	threads   int
)

// Processor is the algorithm to fetch and compare data from indexer and algod
type Processor interface {
	ProcessAddress(addr string, config params) error
}

func init() {
	errorLog = log.New(os.Stderr, "", 1)
}

func main() {
	flag.StringVar(&config.algodUrl, "algod-url", "", "Algod url.")
	flag.StringVar(&config.algodToken, "algod-token", "", "Algod token.")
	flag.StringVar(&config.indexerUrl, "indexer-url", "", "Indexer url.")
	flag.StringVar(&config.indexerToken, "indexer-token", "", "Indexer toke.n")
	flag.StringVar(&addr, "addr", "", "If provided validate a single address instead of reading Stdin.")
	flag.IntVar(&config.retries, "retries", 0, "Number of retry attempts when a difference is detected.")
	flag.IntVar(&threads, "threads", 10, "Number of worker threads to initialize.")
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

	//var processor Processor = GenericProcessor{}
	var processor Processor = MakeStructProcessor(config)

	// Process a single address
	if len(addr) != 0 {
		err := processor.ProcessAddress(addr, config)
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	// Otherwise start the threads and read standard input.
	var wg sync.WaitGroup
	work := make(chan string, 1000000)

	// Start the workers
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range work {
				err := processor.ProcessAddress(addr, config)
				if err != nil {
					fmt.Println(err)
				}
			}
		}()
	}

	// Read work from stdin and pass along to workers
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		work <- scanner.Text()
	}
	close(work)

	// Wait for workers to finish.
	wg.Wait()
}
