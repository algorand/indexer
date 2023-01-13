package pipeline

import log "github.com/sirupsen/logrus"

// HandlePanic function to log panics in a common way
func HandlePanic(logger *log.Logger) {
	if r := recover(); r != nil {
		logger.Panicf("conduit pipeline experienced a panic: %v", r)
	}
}
