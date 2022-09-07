package disabledeadlock

import "github.com/algorand/go-deadlock"

func init() {
	// disable go-deadlock detection
	deadlock.Opts.Disable = true
}
