package core

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sdk_types "github.com/algorand/go-algorand-sdk/types"

	"github.com/algorand/indexer/api"
)

// Params are the program arguments which need to be passed between objects.
type Params struct {
	AlgodURL     string
	AlgodToken   string
	IndexerURL   string
	IndexerToken string
	Retries      int
	RetryDelayMS int
}

func init() {
	ErrorLog = log.New(os.Stderr, "", 1)
	ErrorLog.SetFlags(0)
}

// ErrorLog is used while the validator is running.
var ErrorLog *log.Logger

// Processor is the algorithm to fetch and compare data from indexer and algod
type Processor interface {
	ProcessAddress(algodData []byte, indexerData []byte) (Result, error)
}

// Skip indicates why something was skipped.
type Skip string

const (
	// NotSkipped is the default value indicated the results are not skipped.
	NotSkipped Skip = ""
	// SkipLimitReached is used when the result is skipped because an account
	// resource limit prevents fetching results.
	SkipLimitReached Skip = "account-limit"
)

// Result is the output of ProcessAddress.
type Result struct {
	// Error is set if there were errors running the test.
	Error      error
	SameRound  bool
	SkipReason Skip

	Equal   bool
	Retries int
	Details *ErrorDetails
}

// ErrorDetails are additional details attached to a result in the event of an error.
type ErrorDetails struct {
	Address string
	Algod   string
	Indexer string
	Diff    []string
}

// ProcessorID is used to select which processor to use for validation.
type ProcessorID int

// ProcessorIDs
const (
	Struct ProcessorID = iota
	Dynamic
	Default = Struct
)

// MakeProcessor initializes the Processor from a ProcessorID
func MakeProcessor(id ProcessorID) (Processor, error) {
	switch id {
	case Struct:
		return StructProcessor{}, nil
	case Dynamic:
		return DynamicProcessor{}, nil
	default:
		return nil, fmt.Errorf("invalid processor selected")
	}
}

// Start runs the validator with data from work and puts results in results.
func Start(work <-chan string, processorID ProcessorID, threads int, config Params, results chan<- Result) {
	defer close(results)

	processor, err := MakeProcessor(processorID)
	if err != nil {
		ErrorLog.Fatalf("invalid processor selected.")
	}

	var wg sync.WaitGroup

	// Start the workers
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range work {
				CallProcessor(processor, addr, config, results)
			}
		}()
	}

	// Wait for workers to finish then close the results.
	wg.Wait()
}

// CallProcessor invokes the processor with a retry mechanism.
func CallProcessor(processor Processor, addrInput string, config Params, results chan<- Result) {
	addr, err := normalizeAddress(addrInput)
	if err != nil {
		results <- resultError(err, addrInput)
		return
	}

	algodDataURL := fmt.Sprintf("%s/v2/accounts/%s", config.AlgodURL, addr)
	indexerDataURL := fmt.Sprintf("%s/v2/accounts/%s", config.IndexerURL, addr)

	// Fetch algod account data outside the retry loop. When the data desynchronizes we'll keep fetching indexer data until it
	// catches up with the first algod account query.
	algodData, err := getData(algodDataURL, config.AlgodToken)
	if err != nil {
		results <- resultError(err, addrInput)
		return
	}

	// Retry loop.
	for i := 0; true; i++ {
		indexerData, err := getData(indexerDataURL, config.IndexerToken)
		if err != nil {
			switch {
			case strings.Contains(string(indexerData), api.ErrResultLimitReached):
				results <- resultSkip(err, addrInput, SkipLimitReached)
			default:
				results <- resultError(err, addrInput)
			}
			return
		}

		result, err := processor.ProcessAddress(algodData, indexerData)
		if err != nil {
			// If there is an error return immediately and cram the error.
			results <- Result{
				Equal:   false,
				Error:   fmt.Errorf("error processing account %s: %v", addr, err),
				Retries: i,
				Details: &ErrorDetails{
					Address: addr,
				},
			}
			return
		}

		if result.Equal || result.SameRound || (i >= config.Retries) {
			// Return when results are equal, or when finished retrying.
			result.Retries = i
			if result.Details != nil {
				result.Details.Address = addr
			}
			results <- result
			return
		}

		// Wait before trying again to allow indexer to catch up to the algod account data.
		time.Sleep(time.Duration(config.RetryDelayMS) * time.Millisecond)
	}
}

// normalizeAddress accepts an algorand address or base64 encoded address and outputs the algorand address
func normalizeAddress(addr string) (string, error) {
	_, err := sdk_types.DecodeAddress(addr)
	if err == nil {
		return addr, nil
	}

	addrBytes, err := base64.StdEncoding.DecodeString(addr)
	if err == nil {
		var address sdk_types.Address
		copy(address[:], addrBytes)
		return address.String(), nil
	}

	return "", fmt.Errorf("unable to decode address")
}

// getData from indexer/algod with optional token.
func getData(url, token string) ([]byte, error) {
	if !strings.HasPrefix(url, "http") {
		url = fmt.Sprintf("http://%s", url)
	}

	auth := fmt.Sprintf("Bearer %s", token)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			ErrorLog.Fatalf("failed to close body: %v", err)
		}
	}()

	data, ioErr := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// We attempted to read the body even though the status was bad.
		// Return the bad status error, and the data if available.
		return data, fmt.Errorf("bad status: %s", resp.Status)
	}

	return data, ioErr
}

func resultError(err error, address string) Result {
	return resultSkip(err, address, NotSkipped)
}

func resultSkip(err error, address string, skip Skip) Result {
	return Result{
		Equal:      false,
		Error:      err,
		SkipReason: skip,
		Retries:    0,
		Details: &ErrorDetails{
			Address: address,
		},
	}
}
