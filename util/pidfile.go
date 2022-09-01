package util

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

// CreateIndexerPidFile creates the pid file at the specified location
func CreateIndexerPidFile(logger *log.Logger, pidFilePath string) error {
	var err error
	logger.Infof("Creating PID file at: %s", pidFilePath)
	fout, err := os.Create(pidFilePath)
	if err != nil {
		err = fmt.Errorf("%s: could not create pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}

	if _, err = fmt.Fprintf(fout, "%d", os.Getpid()); err != nil {
		err = fmt.Errorf("%s: could not write pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}

	err = fout.Close()
	if err != nil {
		err = fmt.Errorf("%s: could not close pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}
	return err
}
