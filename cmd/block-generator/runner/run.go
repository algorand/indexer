package runner

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// Load the postgres sql.DB implementation
	_ "github.com/lib/pq"

	"github.com/algorand/indexer/cmd/block-generator/generator"
)

// Args are all the things needed to run a performance test.
type Args struct {
	// Path is a directory when passed to RunBatch, otherwise a file path.
	Path                     string
	IndexerBinary            string
	IndexerPort              uint64
	PostgresConnectionString string
	RunDuration              time.Duration
	ReportDirectory          string
}

// Run is a public helper to run the tests.
// The test will run against the generator configuration file specified by 'args.Path'.
// If 'args.Path' is a directory it should contain generator configuration files, a test will run using each file.
func Run(args Args) error {
	if _, err := os.Stat(args.ReportDirectory); os.IsExist(err) {
		return fmt.Errorf("report directory '%s' already exists", args.ReportDirectory)
	}
	os.Mkdir(args.ReportDirectory, os.ModeDir|os.ModePerm)

	return filepath.Walk(args.Path, func(path string, info os.FileInfo, err error) error {
		// Ignore the directory
		if info.IsDir() {
			return nil
		}
		runnerArgs := args
		runnerArgs.Path = path
		fmt.Printf("Running test for configuration '%s'\n", path)
		return runnerArgs.run()
	})
}

func (r *Args) run() error {
	// Start services
	algodNet := fmt.Sprintf("localhost:%d", 11112)
	indexerNet := fmt.Sprintf("localhost:%d", r.IndexerPort)
	generatorShutdownFunc := startGenerator(r.Path, algodNet)
	indexerShutdownFunc, err := startIndexer(r.IndexerBinary, algodNet, indexerNet, r.PostgresConnectionString)
	if err != nil {
		return fmt.Errorf("failed to start indexer: %w", err)
	}

	// Run the test, collecting results.
	if err := r.runTest(indexerNet, algodNet); err != nil {
		return err
	}

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

// Helper to record the import rate.
func recordDataToFile(entry Entry, key string, out *os.File) error {
	sum := 0.0
	count := 0.0

	for _, metric := range entry.Data {
		var err error
		if strings.HasPrefix(metric, "indexer_daemon_import_time_sec_sum") {
			val := strings.Split(metric, " ")[1]
			sum, err = strconv.ParseFloat(val, 64)
		} else if strings.HasPrefix(metric, "indexer_daemon_import_time_sec_count") {
			val := strings.Split(metric, " ")[1]
			count, err = strconv.ParseFloat(val, 64)
		}
		if err != nil {
			return fmt.Errorf("unable to parse metric '%s': %w", metric, err)
		}
	}
	rate := sum / count

	msg := fmt.Sprintf("%s:%f\n", key, rate)
	if _, err := out.WriteString(msg); err != nil {
		return fmt.Errorf("unable to write metric '%s': %w", key, err)
	}
	return nil
}

// Run the test for 'RunDuration', collect metrics and write them to the 'ReportDirectory'
func (r *Args) runTest(indexerURL string, generatorURL string) error {
	collector := &MetricsCollector{MetricsURL: fmt.Sprintf("http://%s/metrics", indexerURL)}

	baseName := filepath.Base(r.Path)
	baseNameNoExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	reportPath := path.Join(r.ReportDirectory, fmt.Sprintf("%s.report", baseNameNoExt))

	report, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("unable to create report: %w", err)
	}
	defer report.Close()

	// Run for r.RunDuration
	start := time.Now()
	for time.Since(start) < r.RunDuration {
		time.Sleep(r.RunDuration / 10)
		if err := collector.Collect("indexer_daemon_import_time_sec"); err != nil {
			return fmt.Errorf("problem collecting metrics: %w", err)
		}
	}
	if err := collector.Collect("indexer_daemon_import_time_sec"); err != nil {
		return fmt.Errorf("problem collecting metrics: %w", err)
	}

	// Collect results.
	durationStr := fmt.Sprintf("test_duration_seconds:%d\n", uint64(r.RunDuration.Seconds()))
	if _, err := report.WriteString(durationStr); err != nil {
		return fmt.Errorf("unable to write duration metric: %w", err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/report", generatorURL))
	if err != nil {
		return fmt.Errorf("generator report query failed")
	}
	defer resp.Body.Close()
	var generatorReport generator.Report
	if err := json.NewDecoder(resp.Body).Decode(&generatorReport); err != nil {
		return fmt.Errorf("problem decoding generator report: %w", err)
	}
	for metric, entry := range generatorReport {
		// Skip this one
		if metric == "genesis" {
			continue
		}
		str := fmt.Sprintf("transaction_%s_total:%d\n", metric, entry.GenerationCount)
		if _, err := report.WriteString(str); err != nil {
			return fmt.Errorf("unable to write transaction_count metric: %w", err)
		}
	}

	// Record a rate from one of the first data points.
	if len(collector.Data) > 5 {
		if err := recordDataToFile(collector.Data[2], "starting_block_import_duration_average_seconds", report); err != nil {
			return err
		}
	}

	// Also record the final one.
	if err := recordDataToFile(collector.Data[len(collector.Data)-1], "final_import_duration_average_seconds", report); err != nil {
		return err
	}

	return nil
}

// startGenerator starts the generator server.
func startGenerator(configFile string, addr string) func() error {
	// Start generator.
	server, done := generator.StartServer(configFile, addr)

	return func() error {
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("failed during generator graceful shutdown: %w", err)
		}

		// Wait for graceful shutdown or crash.
		select {
		case <-done:
			// continue
			return nil
		case <-time.After(10 * time.Second):
			return fmt.Errorf("failed to gracefully shutdown generator")
		}
	}
}

// startIndexer resets the postgres database and executes the indexer binary. It performs some simple verification to
// ensure that the service has started properly.
func startIndexer(indexerBinary string, algodNet string, indexerNet string, postgresConnectionString string) (func() error, error) {
	{
		db, err := sql.Open("postgres", postgresConnectionString)
		if err != nil {
			return nil, fmt.Errorf("postgres connection string did not work: %w", err)
		}
		if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
			return nil, fmt.Errorf("unable to reset postgres DB: %w", err)
		}
		if err := db.Close(); err != nil {
			return nil, fmt.Errorf("unable to close database handle: %w", err)
		}
	}

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
	time.Sleep(5 * time.Second)
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

		return nil
	}, nil
}
