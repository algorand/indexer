package loggers

import (
	"io"
	"sync"

	log "github.com/sirupsen/logrus"
)

// MT a logger that is created via the LoggerManager and is thread safe (MT == multi-threaded)
type MT struct {
	*log.Logger
}

// LoggerManager a manager that can produce loggers that are synchronized internally
type LoggerManager struct {
	internalWriter ThreadSafeWriter
}

// AdaptLogger will take a logger instance and wrap it for synchronization
func AdaptLogger(log *log.Logger) *LoggerManager {
	return MakeLoggerManager(log.Out)
}

// MakeLogger returns a logger that is synchronized with the internal mutex
func (l *LoggerManager) MakeLogger() *MT {
	logger := log.New()
	logger.SetOutput(l.internalWriter)
	return &MT{logger}
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
