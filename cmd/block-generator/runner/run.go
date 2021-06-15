package runner

import (
	"context"
	"fmt"
	"github.com/algorand/indexer/cmd/block-generator/generator"
	"os"
	"path/filepath"
	"time"

	"github.com/algorand/indexer/util"
)

// RunnerArgs are all the things needed to run a performance test.
type RunnerArgs struct {
	// Path is a directory when passed to RunBatch, otherwise a file path.
	Path string
	IndexerBinary string
	IndexerPort uint64
	PostgresConnectionString string
	RunDuration time.Duration
	ReportDirectory string

	indexerPort uint64
}

// Run is a public helper run the tests.
func Run(args RunnerArgs) error {
	stat, err := os.Stat(args.Path)
	util.MaybeFail(err, "Unable to check path.")

	// Batch mode
	if stat.IsDir() {
		return filepath.Walk(args.Path, func(path string, info os.FileInfo, err error) error {
			runnerArgs := args
			runnerArgs.Path = path
			return runnerArgs.run()
		})
	}

	// Single file mode
	return args.run()
}

func (r *RunnerArgs) run() error {
	port := 11112
	server, done := generator.StartServer(r.Path, port)

	time.Sleep(r.RunDuration)

	if err := server.Shutdown(context.Background()); err != nil {
		panic(err) // failure/timeout shutting down the server gracefully
	}

	// Wait for graceful shutdown or crash.
	select {
		case <- done:
			// continue
		case <- time.After(10 * time.Second):
			fmt.Println("Failed to gracefully shutdown generator.")
			os.Exit(1)
	}

	return nil
}
