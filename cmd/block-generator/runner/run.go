package runner

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	// Load the postgres sql.DB implementation
	_ "github.com/lib/pq"

	"github.com/algorand/indexer/cmd/block-generator/generator"
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
	pathStat, err := os.Stat(args.Path)
	util.MaybeFail(err, "Unable to check path.")

	reportDirStat, err := os.Stat(args.ReportDirectory)
	util.MaybeFail(err, "Unable to check report directory.")
	if !reportDirStat.IsDir() {
		return fmt.Errorf("report directory '%s' is not a directory", args.ReportDirectory)
	}

	// Batch mode
	if pathStat.IsDir() {
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
	port := uint64(11112)

	// Start services
	generatorShutdownFunc := startGenerator(r.Path, port)
	indexerShutdownFunc, err := startIndexer(r.IndexerBinary, port, r.IndexerPort, r.PostgresConnectionString)
	if err != nil {
		return fmt.Errorf("failed to start indexer: %w", err)
	}

	// Run the test, collecting results.
	r.runTest()

	// Shutdown generator.
	if err := generatorShutdownFunc(); err != nil {
		return err
	}

	// Shutdown indexer
	if err := indexerShutdownFunc(); err != nil {
		return err
	}

	return nil
}

func (r *RunnerArgs) runTest() error {
	time.Sleep(r.RunDuration)
	return nil
}

// startGenerator starts the generator server.
func startGenerator(configFile string, port uint64) func() error {
	// Start generator.
	server, done := generator.StartServer(configFile, port)

	return func() error {
		if err := server.Shutdown(context.Background()); err != nil {
			panic(err) // failure/timeout shutting down the server gracefully
		}

		// Wait for graceful shutdown or crash.
		select {
		case <- done:
			// continue
			return nil
		case <- time.After(10 * time.Second):
			return fmt.Errorf("failed to gracefully shutdown generator")
		}
	}
}

// startIndexer resets the postgres database and executes the indexer binary. It performs some simple verification to
// ensure that the service has started properly.
func startIndexer(indexerBinary string, algodPort uint64, indexerPort uint64, postgresConnectionString string) (func() error, error) {
	{
		db, err := sql.Open("postgres", postgresConnectionString)
		if err != nil {
			return nil, fmt.Errorf("postgres connection string did not work: %w", err)
		}
		db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
		db.Close()
	}

	time.Sleep(250 * time.Millisecond)

	algodNet := fmt.Sprintf("localhost:%d", algodPort)
	indexerNet := fmt.Sprintf("localhost:%d", indexerPort)
	cmd := exec.Command(
		indexerBinary,
		"daemon",
		"--algod-net", algodNet,
		"--algod-token", "secure-token-here",
		"--metrics-mode", "VERBOSE",
		"--postgres", postgresConnectionString,
		"--server", indexerNet)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failure calling Start(): %w", err)
	}

	// Ensure that the health endpoint can be queried.
	// The service should start very quickly because the DB is empty.
	time.Sleep(250 * time.Millisecond)
	resp, err := http.Get(fmt.Sprintf("http://%s/health", indexerNet))
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdout:\n%s\n", stdout.String())
		fmt.Fprintf(os.Stderr, "stderr:\n%s\n", stderr.String())
		return nil, fmt.Errorf("the process failed to start properly, health endpoint query failed")
	}
	resp.Body.Close()

	return func() error {
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill indexer process: %w", err)
		}

		// Clear postgres DB
		return nil
	}, nil
}
