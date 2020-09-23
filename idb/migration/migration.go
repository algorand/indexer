package migration

import (
	"errors"
	"fmt"
)

var DuplicateIDErr = errors.New("Duplicate ID detected.")

const StatusInitializing = "Migration initializing"
const StatusComplete = "Migrations Complete"
const StatusActivePrefix = "Active migration: "
const StatusErrorPrefix = "error during migration "

// A migration function should take care of writing back to metastate migration row
type Handler func() error

type Task struct {
	MigrationId int

	Handler Handler

	// The system should wait for this migration to finish before trying to import new data or serve data.
	PreventStartup bool

	// Description of the migration
	Description string
}

func RunMigrations(errChan chan <-error, statusChan chan <-string, doneChan chan <-struct{}, unblockChan chan <-struct{}, migrations ...Task) error {
	set := make(map[int]bool)

	// Search for final blocking migration, if any.
	unblockAfter := 0
	for _, migration := range migrations {
		if migration.PreventStartup {
			unblockAfter = migration.MigrationId
		}

		// Prevent duplicate IDs
		if set[migration.MigrationId] {
			return DuplicateIDErr
		}
		set[migration.MigrationId] = true
	}

	// Run migrations in another go routine.
	go func() {
		blocking := true

		// No need to block, send the unblock message right away
		if unblockAfter == 0 {
			unblockChan <- struct{}{}
			blocking = false
		}

		statusChan <- StatusInitializing

		for _, task := range migrations {
			statusChan <- StatusActivePrefix + task.Description
			err := task.Handler()

			if err != nil {
				err := fmt.Errorf("%s%d (%s): %v", StatusErrorPrefix, task.MigrationId, task.Description, err)
				if blocking {
					unblockChan <- struct{}{}
				}
				statusChan <- err.Error()
				errChan <- err
				doneChan <- struct{}{}
				return
			}

			if unblockAfter == task.MigrationId {
				unblockChan <- struct{}{}
				blocking = false
			}
		}

		statusChan <- StatusComplete
		if blocking {
			unblockChan <- struct{}{}
		}
		doneChan <- struct{}{}
	}()

	return nil
}
