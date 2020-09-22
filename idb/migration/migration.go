package migration

import (
	"fmt"
)

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

func RunMigrations(errChan chan <-error, statusChan chan <-string, doneChan chan <-struct{}, unblockChan chan <-struct{}, migrations ...Task) {
	statusChan <- "Migrations initializing"

	// Search for final blocking migration, if any.
	unblockAfter := 0
	for _, migration := range migrations {
		if migration.PreventStartup {
			unblockAfter = migration.MigrationId
		}
	}

	// Run migrations in another go routine.
	go func() {
		for _, task := range migrations {
			statusChan <- "Active migration: " + task.Description
			err := task.Handler()

			if err != nil {
				err := fmt.Errorf("error during migration %d (%s): %v", task.MigrationId, task.Description, err)
				unblockChan <- struct{}{}
				statusChan <- err.Error()
				errChan <- err
				doneChan <- struct{}{}
				return
			}

			if unblockAfter == task.MigrationId {
				unblockChan <- struct{}{}
			}
		}

		statusChan <- "Migrations Complete"
		unblockChan <- struct{}{}
		doneChan <- struct{}{}
	}()

	// No need to block, send the unblock message right away
	if unblockAfter == 0 {
		unblockChan <- struct{}{}
		return
	}
}
