package loggers

import (
	"fmt"
	"io"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/indexer/conduit/pipeline"
)

// LoggerManager a manager that can produce loggers that are synchronized internally
type LoggerManager struct {
	internalWriter ThreadSafeWriter
}

// MakeRootLogger returns a logger that is synchronized with the internal mutex
func (l *LoggerManager) MakeRootLogger(level log.Level, logFile string) (*log.Logger, error) {
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
	logger.SetOutput(l.internalWriter)

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, fmt.Errorf("runConduitCmdWithConfig(): %w", err)
		}
		l.internalWriter.Mutex.Lock()
		defer l.internalWriter.Mutex.Unlock()
		l.internalWriter.Writer = f
	}
	return logger, nil
}

// MakeLoggerManager returns a logger manager
func MakeLoggerManager(writer io.Writer) *LoggerManager {
	return &LoggerManager{
		internalWriter: ThreadSafeWriter{
			Writer: writer,
			Mutex:  &sync.Mutex{},
		},
	}
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
