package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/v3/types"
	"github.com/algorand/indexer/v3/util/test"
	"github.com/stretchr/testify/require"
)

const benchmarkDuration = 45 * time.Second

// gnomock's defaultStopTimeoutSec is 1 seconds after which
// the container is forcefully killed. So wait a little longer
// to make sure the container is cleaned up.
const waitForContainerCleanup = 1500 * time.Millisecond

var benchmarkLogger *log.Logger

func init() {
	benchmarkLogger = log.New()
	benchmarkLogger.SetFormatter(&log.JSONFormatter{})
	benchmarkLogger.SetOutput(os.Stdout)
	benchmarkLogger.SetLevel(log.ErrorLevel)
}

type benchmarkCase struct {
	name           string
	pgVersion      string
	maxConns       int32
	isolationLevel pgx.TxIsoLevel
	batchSize      uint32
	scenario       string
	blockSize      uint
}

const blockPathFormat = "test_resources/file_exported_blocks/5_%s.%d.msgp.gz"

// pg version 16 turned out to be ~ 70% __FASTER__ for payments.
// However, it also crashes unpredictably!!!
// var pgVersions = []string{"14", "15", "16beta3"}

var pgVersions = []string{"15"}
var maxConnss = []int32{4, 8, 12, 16, 20}
var isoLevels = []pgx.TxIsoLevel{pgx.Serializable} //pgx.ReadUncommitted} //, pgx.ReadCommitted} // pgx.RepeatableRead, pgx.ReadCommitted} //, pgx.ReadUncommitted}
var batchSizes = []uint32{1_000, 2_000, 4_000}                      //{16_000, 12_000, 8_000, 4_000, 2000, 1000} //25_000 10_000, 2_500, 1_000, 250, 100}
var scenarios = []string{"organic", "payment", "stress"}
var blockSizes = []uint{25_000} //, 50_000}

var benchCases []benchmarkCase
var r *rand.Rand

func init() {
	for _, pgVersion := range pgVersions {
		for _, maxConns := range maxConnss {
			for _, isoLevel := range isoLevels {
				for _, batchSize := range batchSizes {
					for _, scenario := range scenarios {
						for _, blockSize := range blockSizes {
							name := fmt.Sprintf(
								"%s_%d_%s_%d-%s_%d",
								pgVersion, maxConns, shortName(isoLevel), batchSize,
								scenario, blockSize,
							)
							benchCases = append(benchCases, benchmarkCase{
								name:           name,
								pgVersion:      pgVersion,
								maxConns:       maxConns,
								isolationLevel: isoLevel,
								batchSize:      batchSize,
								scenario:       scenario,
								blockSize:      blockSize,
							})
						}
					}
				}
			}
		}
	}

	r = rand.New(rand.NewSource(time.Now().UnixNano()))
}

type exportStats struct {
	trials        uint
	setupDuration time.Duration
	simDuration   time.Duration
	fullRounds    uint
	bytes         uint64
	approxTxns    uint
}

func add(x, y exportStats) exportStats {
	return exportStats{
		trials:        x.trials + y.trials,
		setupDuration: x.setupDuration + y.setupDuration,
		simDuration:   x.simDuration + y.simDuration,
		fullRounds:    x.fullRounds + y.fullRounds,
		bytes:         x.bytes + y.bytes,
		approxTxns:    x.approxTxns + y.approxTxns,
	}
}

func reportMetrics(b *testing.B, stats exportStats) {
	b.ReportMetric(float64(stats.setupDuration.Seconds()), "setupTime")
	b.ReportMetric(float64(stats.simDuration.Seconds()), "simTime")
	b.ReportMetric(float64(stats.bytes), "bytes")
	b.ReportMetric(float64(stats.bytes)/stats.simDuration.Seconds(), "bytes/sec")
	b.ReportMetric(float64(stats.approxTxns), "approxTxns")
	b.ReportMetric(float64(stats.approxTxns)/stats.simDuration.Seconds(), "approxTxns/sec")
	b.ReportMetric(float64(stats.fullRounds), "rounds")
	b.ReportMetric(float64(stats.fullRounds)/stats.simDuration.Seconds(), "rounds/sec")
}

func simulateExport(b *testing.B, vb *types.ValidatedBlock, size uint64, db *IndexerDb, ccf context.CancelCauseFunc, stats *exportStats) {
	start := time.Now()

	round := sdk.Round(1)
	for ; time.Since(start) < benchmarkDuration; round++ {
		vb.Block.Round = round
		err := db.AddBlock(vb)
		require.NoError(b, err)
	}
	stats.fullRounds = uint(round)
	ccf(fmt.Errorf("simulation complete after %d rounds. runtime=%s > behcnmark duration=%s", round, time.Since(start), benchmarkDuration))
}

func BenchmarkExport(b *testing.B) {
	r.Shuffle(len(benchCases), func(i, j int) {
		benchCases[i], benchCases[j] = benchCases[j], benchCases[i]
	})
	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			stats := exportStats{}
			blockFile := fmt.Sprintf(blockPathFormat, bc.scenario, bc.blockSize)
			vb, size, err := test.ReadConduitBlockFromFile(blockFile)
			require.NoError(b, err, bc.name)

			for i := 0; i < b.N; i++ {
				setupStart := time.Now()
				tuning := TuningParams{
					PgxOpts:   pgx.TxOptions{IsoLevel: bc.isolationLevel},
					BatchSize: bc.batchSize,
				}
				db, shutdownFunc := setupIdbWithPgVersion(b, test.MakeGenesis(), bc.pgVersion, &bc.maxConns, &tuning, benchmarkLogger)
				ctx, ccf := context.WithCancelCause(context.Background())
				trialStats := exportStats{
					trials:        1,
					setupDuration: time.Since(setupStart),
				}

				b.StartTimer()
				go simulateExport(b, &vb, size, db, ccf, &trialStats)

				<-ctx.Done()
				b.StopTimer()
				b.Logf("trial complete because: %v\n", context.Cause(ctx))

				shutdownFunc()

				trialStats.bytes = size * uint64(trialStats.fullRounds)
				trialStats.approxTxns = bc.blockSize * trialStats.fullRounds
				trialStats.simDuration = b.Elapsed()

				reportMetrics(b, trialStats)
				stats = add(stats, trialStats)

				time.Sleep(waitForContainerCleanup)
			}
			b.Logf("benchmark: %#v, stats: %#v", bc, stats)

		})
	}
}

// BenchmarkRead is a sanity check that only benchmarks the block reading functionality
func BenchmarkRead(b *testing.B) {
	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			blockFile := fmt.Sprintf(blockPathFormat, bc.scenario, bc.blockSize)
			vb, size, err := test.ReadConduitBlockFromFile(blockFile)
			require.NoError(b, err, bc.name)
			require.Equal(b, sdk.Round(5), vb.Block.Round, bc.name)

			secs := b.Elapsed().Seconds()
			bps := float64(size) / secs
			b.ReportMetric(bps, "bytes/sec")
			b.ReportMetric(float64(size), "bytes")
		})
	}

}
