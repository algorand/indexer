package conduit

import "github.com/algorand/indexer/loggers"

// HandlePanic function to log panics in a common way
func HandlePanic(logger *loggers.MT) {
	if r := recover(); r != nil {
		logger.Panicf("conduit pipeline experienced a panic: %v", r)
	}
}
