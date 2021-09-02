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
	"syscall"
	"time"

	// Load the postgres sql.DB implementation
	_ "github.com/lib/pq"

	"github.com/algorand/indexer/cmd/block-generator/generator"
	"github.com/algorand/indexer/util"
	"github.com/algorand/indexer/util/metrics"
)

// Args are all the things needed to run a performance test.
type Args struct {
	// Path is a directory when passed to RunBatch, otherwise a file path.
	Path                     string
	IndexerBinary            string
	IndexerPort              uint64
	PostgresConnectionString string
	RunDuration              time.Duration
	LogLevel                 string
	ReportDirectory          string
}

// Run is a public helper to run the tests.
// The test will run against the generator configuration file specified by 'args.Path'.
// If 'args.Path' is a directory it should contain generator configuration files, a test will run using each file.
func Run(args Args) error {
	if _, err := os.Stat(args.ReportDirectory); !os.IsNotExist(err) {
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
	baseName := filepath.Base(r.Path)
	baseNameNoExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	reportfile := path.Join(r.ReportDirectory, fmt.Sprintf("%s.report", baseNameNoExt))
	logfile := path.Join(r.ReportDirectory, fmt.Sprintf("%s.indexer-log", baseNameNoExt))

	// Start services
	algodNet := fmt.Sprintf("localhost:%d", 11112)
	indexerNet := fmt.Sprintf("localhost:%d", r.IndexerPort)
	generatorShutdownFunc := startGenerator(r.Path, algodNet)

	indexerShutdownFunc, err := startIndexer(logfile, r.LogLevel, r.IndexerBinary, algodNet, indexerNet, r.PostgresConnectionString)
	if err != nil {
		return fmt.Errorf("failed to start indexer: %w", err)
	}

	// Run the test, collecting results.
	if err := r.runTest(reportfile, indexerNet, algodNet); err != nil {
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

type metricType int

const (
	rate metricType = iota
	intTotal
	floatTotal
)

// Helper to record metrics. Supports rates (sum/count) and counters.
func recordDataToFile(start time.Time, entry Entry, prefix string, out *os.File) error {
	var writeErrors strings.Builder
	var writeErr error
	record := func(prefix2, name string, t metricType) {
		key := fmt.Sprintf("%s%s_%s", prefix, prefix2, name)
		if err := recordMetricToFile(entry, key, name, t, out); err != nil {
			writeErr = err
			if writeErrors.Len() > 0 {
				writeErrors.WriteString(", ")
			}
			writeErrors.WriteString(name)
		}
	}

	record("_average", metrics.BlockImportTimeName, rate)
	record("_cumulative", metrics.BlockImportTimeName, floatTotal)
	record("_average", metrics.ImportedTxnsPerBlockName, rate)
	record("_cumulative", metrics.ImportedTxnsPerBlockName, intTotal)
	record("_average", metrics.BlockUploadTimeName, rate)
	record("_cumulative", metrics.BlockUploadTimeName, floatTotal)
	record("", metrics.ImportedRoundGaugeName, intTotal)

	if writeErrors.Len() > 0 {
		return fmt.Errorf("error writing metrics (%s): %w", writeErrors.String(), writeErr)
	}

	// Calculate import transactions per second.
	totalTxn, err := getMetric(entry, metrics.ImportedTxnsPerBlockName, false)
	if err != nil {
		return err
	}

	importTimeS, err := getMetric(entry, metrics.BlockImportTimeName, false)
	if err != nil {
		return err
	}
	tps := totalTxn / importTimeS
	key := "overall_transactions_per_second"
	msg := fmt.Sprintf("%s_%s:%.2f\n", prefix, key, tps)
	if _, err := out.WriteString(msg); err != nil {
		return fmt.Errorf("unable to write metric '%s': %w", key, err)
	}

	// Uptime
	key = "uptime_seconds"
	msg = fmt.Sprintf("%s_%s:%.2f\n", prefix, key, time.Since(start).Seconds())
	if _, err := out.WriteString(msg); err != nil {
		return fmt.Errorf("unable to write metric '%s': %w", key, err)
	}

	return nil
}

func recordMetricToFile(entry Entry, outputKey, metricSuffix string, t metricType, out *os.File) error {
	value, err := getMetric(entry, metricSuffix, t == rate)
	if err != nil {
		return err
	}

	var msg string
	if t == intTotal {
		msg = fmt.Sprintf("%s:%d\n", outputKey, uint64(value))
	} else {
		msg = fmt.Sprintf("%s:%.2f\n", outputKey, value)
	}

	if _, err := out.WriteString(msg); err != nil {
		return fmt.Errorf("unable to write metric '%s': %w", outputKey, err)
	}

	return nil
}

func getMetric(entry Entry, suffix string, rateMetric bool) (float64, error) {
	total := 0.0
	sum := 0.0
	count := 0.0
	hasSum := false
	hasCount := false
	hasTotal := false

	for _, metric := range entry.Data {
		var err error

		if strings.Contains(metric, suffix) {
			split := strings.Split(metric, " ")
			if len(split) != 2 {
				return 0.0, fmt.Errorf("unknown metric format, expected 'key value' received: %s", metric)
			}

			// Check for _sum / _count for summary (rateMetric) metrics.
			// Otherwise grab the total value.
			if strings.HasSuffix(split[0], "_sum") {
				sum, err = strconv.ParseFloat(split[1], 64)
				hasSum = true
			} else if strings.HasSuffix(split[0], "_count") {
				count, err = strconv.ParseFloat(split[1], 64)
				hasCount = true
			} else if strings.HasSuffix(split[0], suffix) {
				total, err = strconv.ParseFloat(split[1], 64)
				hasTotal = true
			}

			if err != nil {
				return 0.0, fmt.Errorf("unable to parse metric '%s': %w", metric, err)
			}

			if rateMetric && hasSum && hasCount {
				return sum / count, nil
			} else if !rateMetric {
				if hasSum {
					return sum, nil
				}
				if hasTotal {
					return total, nil
				}
			}
		}
	}

	return 0.0, fmt.Errorf("metric incomplete or not found: %s", suffix)
}

// Run the test for 'RunDuration', collect metrics and write them to the 'ReportDirectory'
func (r *Args) runTest(reportfile string, indexerURL string, generatorURL string) error {
	collector := &MetricsCollector{MetricsURL: fmt.Sprintf("http://%s/metrics", indexerURL)}

	report, err := os.Create(reportfile)
	if err != nil {
		return fmt.Errorf("unable to create report: %w", err)
	}
	defer report.Close()

	// Run for r.RunDuration
	start := time.Now()
	for time.Since(start) < r.RunDuration {
		time.Sleep(r.RunDuration / 10)

		if err := collector.Collect(metrics.AllMetricNames...); err != nil {
			return fmt.Errorf("problem collecting metrics: %w", err)
		}
	}
	if err := collector.Collect(metrics.AllMetricNames...); err != nil {
		return fmt.Errorf("problem collecting metrics: %w", err)
	}

	// Collect results.
	durationStr := fmt.Sprintf("test_duration_seconds:%d\ntest_duration_actual_seconds:%f\n",
		uint64(r.RunDuration.Seconds()),
		time.Since(start).Seconds())
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
		if err := recordDataToFile(start, collector.Data[2], "early", report); err != nil {
			return err
		}
	}

	// Also record the final one.
	if err := recordDataToFile(start, collector.Data[len(collector.Data)-1], "final", report); err != nil {
		return err
	}

	return nil
}

// startGenerator starts the generator server.
func startGenerator(configFile string, addr string) func() error {
	// Start generator.
	server := generator.MakeServer(configFile, addr)

	// Start the server
	go func() {
		// always returns error. ErrServerClosed on graceful close
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			util.MaybeFail(err, "ListenAndServe() failure to start with config file '%s'", configFile)
		}
	}()

	return func() error {
		// Shutdown blocks until the server has stopped.
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("failed during generator graceful shutdown: %w", err)
		}
		return nil
	}
}

// startIndexer resets the postgres database and executes the indexer binary. It performs some simple verification to
// ensure that the service has started properly.
func startIndexer(logfile string, loglevel string, indexerBinary string, algodNet string, indexerNet string, postgresConnectionString string) (func() error, error) {
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
		"--server", indexerNet,
		"--logfile", logfile,
		"--loglevel", loglevel)

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
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to kill indexer process: %w", err)
		}

		return nil
	}, nil
}
