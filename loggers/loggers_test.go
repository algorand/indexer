package loggers

import (
	"encoding/json"
	"math/rand"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeIoWriter struct {
	Entries []string
}

func (f *FakeIoWriter) Write(p []byte) (n int, err error) {
	f.Entries = append(f.Entries, string(p))
	return len(p), nil
}

func TestLogToFile(t *testing.T) {
	logfile := path.Join(t.TempDir(), "mylogfile.txt")
	require.NoFileExists(t, logfile)
	logger, err := MakeThreadSafeLogger(log.InfoLevel, logfile)
	require.NoError(t, err)

	testString := "1234abcd"
	logger.Infof(testString)
	assert.FileExists(t, logfile)
	data, err := os.ReadFile(logfile)
	assert.Contains(t, string(data), testString)
	lines := strings.Split(string(data), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "", lines[1])
}

// TestThreadSafetyOfLogger ensures that multiple threads writing to a single source
// don't get corrupted
func TestThreadSafetyOfLogger(t *testing.T) {
	var atomicInt int64 = 0

	fakeWriter := FakeIoWriter{}

	const numberOfWritesPerLogger = 20
	const numberOfLoggers = 15

	var wg sync.WaitGroup
	wg.Add(numberOfLoggers)
	writer := ThreadSafeWriter{
		Writer: &fakeWriter,
		Mutex:  &sync.Mutex{},
	}

	// Launch go routines
	for i := 0; i < numberOfLoggers; i++ {
		go func() {
			// Sleep a random number of milliseconds before and after to test
			// that creating a logger doesn't affect thread-safety
			time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
			l := log.New()
			l.SetOutput(writer)
			l.SetFormatter(&log.JSONFormatter{
				// We want to disable timestamps to stop logrus from sorting our output
				DisableTimestamp: true,
			})
			time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

			for j := 0; j < numberOfWritesPerLogger; j++ {

				// Atomically adds 1 and returns new value
				localInt := atomic.AddInt64(&atomicInt, 1)
				l.Infof("%d", localInt)

			}
			wg.Done()
		}()

	}
	wg.Wait()

	assert.Equal(t, len(fakeWriter.Entries), numberOfLoggers*numberOfWritesPerLogger)

	// We can't assume that the writes are in order since the call to atomically update
	// and log are not atomic *together*...just independently.  So we test that all
	// numbers are present with a map and have no duplicates
	numMap := make(map[string]bool)

	for i := 0; i < numberOfLoggers*numberOfWritesPerLogger; i++ {
		var jsonText map[string]interface{}
		err := json.Unmarshal([]byte(fakeWriter.Entries[i]), &jsonText)
		assert.NoError(t, err)

		sourceString := jsonText["msg"].(string)

		_, ok := numMap[sourceString]
		// We shouldn't have seen this before
		assert.False(t, ok)

		numMap[sourceString] = true

	}

	assert.Equal(t, len(numMap), numberOfLoggers*numberOfWritesPerLogger)

}
