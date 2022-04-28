package core

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// ValidatorCmd is the account validator utility.
var ValidatorCmd *cobra.Command

func init() {
	var (
		config       Params
		addr         string
		threads      int
		processorNum int
		printCurl    bool
		errorLogFile string
	)

	ValidatorCmd = &cobra.Command{
		Use:   "validator",
		Short: "validator",
		Long:  "Compare algod and indexer to each other and report any discrepencies.",
		Run: func(cmd *cobra.Command, _ []string) {
			run(config, errorLogFile, addr, threads, processorNum, printCurl)
		},
	}

	ValidatorCmd.Flags().StringVar(&config.AlgodURL, "algod-url", "", "Algod url.")
	ValidatorCmd.MarkFlagRequired("algod-url")
	ValidatorCmd.Flags().StringVar(&config.AlgodToken, "algod-token", "", "Algod token.")
	ValidatorCmd.Flags().StringVar(&config.IndexerURL, "indexer-url", "", "Indexer url.")
	ValidatorCmd.MarkFlagRequired("indexer-url")
	ValidatorCmd.Flags().StringVar(&config.IndexerToken, "indexer-token", "", "Indexer token.")
	ValidatorCmd.Flags().IntVarP(&config.Retries, "retries", "", 5, "Number of retry attempts when a difference is detected.")
	ValidatorCmd.Flags().IntVarP(&config.RetryDelayMS, "retry-delay", "", 1000, "Time in milliseconds to sleep between retries.")
	ValidatorCmd.Flags().StringVar(&addr, "addr", "", "If provided validate a single address instead of reading Stdin.")
	ValidatorCmd.Flags().IntVar(&threads, "threads", 4, "Number of worker threads to initialize.")
	ValidatorCmd.Flags().IntVar(&processorNum, "processor", 0, "Choose compare algorithm [0 = Struct, 1 = Reflection]")
	ValidatorCmd.Flags().BoolVar(&printCurl, "print-commands", false, "Print curl commands, including tokens, to query algod and indexer.")
	ValidatorCmd.Flags().StringVarP(&errorLogFile, "error-log-file", "e", "", "When specified, error messages are written to this file instead of to stderr.")
}

func run(config Params, errorLogFile, addr string, threads int, processorNum int, printCurl bool) {
	if len(config.AlgodURL) == 0 {
		ErrorLog.Fatalf("algod-url parameter is required.")
	}
	if len(config.AlgodToken) == 0 {
		ErrorLog.Fatalf("algod-token parameter is required.")
	}
	if len(config.IndexerURL) == 0 {
		ErrorLog.Fatalf("indexer-url parameter is required.")
	}

	if errorLogFile != "" {
		_, err := os.Stat(errorLogFile)
		if !os.IsNotExist(err) {
			ErrorLog.Fatalf("Error log already exists: %s", errorLogFile)
		}

		errorWriter, err := os.OpenFile(errorLogFile, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			ErrorLog.Fatalf("Unable to open error log file: %s", err)
		}
		ErrorLog.SetOutput(errorWriter)
	}

	results := make(chan Result, 10)

	go func() {
		if len(addr) != 0 {
			processor, err := MakeProcessor(ProcessorID(processorNum))
			if err != nil {
				ErrorLog.Fatalf("%s.\n", err)
			}

			// Process a single address
			CallProcessor(processor, addr, config, results)
			close(results)
		} else {
			// Process from stdin
			start(ProcessorID(processorNum), threads, config, results)
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
func start(processorID ProcessorID, threads int, config Params, results chan<- Result) {
	work := make(chan string, 100*threads)

	// Read addresses from stdin and pass along to workers
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			work <- scanner.Text()
		}
		close(work)
	}()

	Start(work, processorID, threads, config, results)
}

// resultChar picks the appropriate status character for the output.
func resultChar(success bool, retries int, skipReason Skip) string {
	if success && retries == 0 {
		return "."
	}
	if success && retries > 9 {
		return fmt.Sprintf("(%d)", retries)
	}
	if success {
		return fmt.Sprintf("%d", retries)
	}

	if skipReason != NotSkipped {
		return "_"
	}

	return "X"
}

// resultsPrinter reads the results channel and prints it to the error log. Returns the number of errors.
func resultsPrinter(config Params, printCurl bool, results <-chan Result) int {
	numResults := 0
	numErrors := 0
	skipCounts := make(map[Skip]uint64)
	numRetries := 0
	startTime := time.Now()

	stats := func() {
		endTime := time.Now()
		duration := endTime.Sub(startTime)
		fmt.Printf("\n\nNumber of errors: [%d / %d]\n", numErrors, numResults)
		fmt.Printf("Skipped (%s): %d\n", SkipLimitReached, skipCounts[SkipLimitReached])
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
		fmt.Printf("%s", resultChar(r.Equal, r.Retries, r.SkipReason))

		numResults++
		numRetries += r.Retries
		if r.Error != nil || !r.Equal {
			if r.SkipReason != NotSkipped {
				skipCounts[r.SkipReason]++
			} else {
				numErrors++
			}
			ErrorLog.Printf("===================================================================")
			ErrorLog.Printf("%s", time.Now().Format("2006-01-02 3:4:5 PM"))
			ErrorLog.Printf("Account: %s", r.Details.Address)
			ErrorLog.Printf("Retries: %d", r.Retries)
			ErrorLog.Printf("Rounds Match: %t", r.SameRound)

			// Print error message if there is one.
			if r.SkipReason != NotSkipped {
				switch r.SkipReason {
				case SkipLimitReached:
					ErrorLog.Printf("Address skipped: too many asset and/or accounts to return\n")
				default:
					ErrorLog.Printf("Address skipped: Unknown reason (%s)\n", r.SkipReason)
				}

			} else if r.Error != nil {
				ErrorLog.Printf("Processor error: %v\n", r.Error)
			} else {
				// Print error details if there are any.
				if r.Details != nil {
					ErrorLog.Printf("Algod Details:\n%s", r.Details.Algod)
					ErrorLog.Printf("Indexer Details:\n%s", r.Details.Indexer)
					ErrorLog.Printf("Differences:")
					for _, diff := range r.Details.Diff {
						ErrorLog.Printf("     - %s", diff)
					}
				}
				// Optionally print curl command.
				if printCurl {
					ErrorLog.Printf("echo 'Algod:'")
					ErrorLog.Printf("curl -q -s -H 'Authorization: Bearer %s' '%s/v2/accounts/%s?pretty'", config.AlgodToken, config.AlgodURL, r.Details.Address)
					ErrorLog.Printf("echo 'Indexer:'")
					ErrorLog.Printf("curl -q -s -H 'Authorization: Bearer %s' '%s/v2/accounts/%s?pretty'", config.IndexerToken, config.IndexerURL, r.Details.Address)
				}
			}
		}
	}

	return numErrors
}
