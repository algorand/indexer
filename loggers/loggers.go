package loggers

import (
	"fmt"
	"io"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit/pipeline"
)

func MakeThreadSafeLoggerWithWriter(level log.Level, writer io.Writer) *log.Logger {
	formatter := pipeline.PluginLogFormatter{
		Formatter: &log.JSONFormatter{
			DisableHTMLEscape: true,
		},
		Type: "Conduit",
		Name: "main",
	}

	logger := log.New()
	logger.SetFormatter(&formatter)
	logger.SetLevel(level)

	logger.SetOutput(ThreadSafeWriter{
		Writer: writer,
		Mutex:  &sync.Mutex{},
	})

	return logger
}

// MakeThreadSafeLogger returns a logger that is synchronized with the internal mutex, if no file is provided write
// to os.Stdout.
func MakeThreadSafeLogger(level log.Level, logFile string) (*log.Logger, error) {
	var writer io.Writer
	// Write to a file or stdout
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, fmt.Errorf("runConduitCmdWithConfig(): %w", err)
		}
		writer = f
	} else {
		writer = os.Stdout
	}

	return MakeThreadSafeLoggerWithWriter(level, writer), nil
}

// ThreadSafeWriter a struct that implements io.Writer in a threadsafe way
type ThreadSafeWriter struct {
	Writer io.Writer
	Mutex  *sync.Mutex
}

// Write writes p bytes with the mutex
func (w ThreadSafeWriter) Write(p []byte) (n int, err error) {
	w.Mutex.Lock()
	defer w.Mutex.Unlock()
	return w.Writer.Write(p)
}
