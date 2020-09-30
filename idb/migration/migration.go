package migration

import (
	"errors"
	"fmt"
	"sync"
)

var DuplicateIDErr = errors.New("duplicate ID detected")
var UnorderedIDErr = errors.New("migration IDs must be in ascending order")

const StatusPending = "Migration pending"
const StatusComplete = "Migrations Complete"
const StatusActivePrefix = "Active migration: "
const StatusErrorPrefix = "error during migration "

// Handler is the function which will be executed to perform the migration for this task.
type Handler func() error

// Task is used to define a migration.
type Task struct {
	// MigrationId is an internal ID that can be used by IndexerDb implementations.
	MigrationId int

	// Handler is the function which will be executed to perform the migration for this task.
	Handler Handler

	// DBUnavailable indicates whether or not queries should be blocked until this task is executed. If there are
	// multiple migrations the migration status should indicate that queries are blocked until all blocking tasks
	// have completed.
	DBUnavailable bool

	// Description should be a human readable description of what the migration does.
	Description string
}

// State is the current status of the migration.
type State struct {
	// Err is the last error which occurred during the migration. On an error the migration should halt.
	Err      error

	// Status is the most recent status message.
	Status   string

	// Running indicates whether or not a migration is currently running.
	Running  bool

	// Blocking indicates that one or more tasks have requested that the DB remain unavailable until they complete.
	Blocking bool
}

// Migration manages the execution of multiple migration tasks and provides a mechanism for concurrent status checks.
type Migration struct {
	mutex      sync.RWMutex
	tasks      []Task
	blockUntil int
	state      State
}

// Broken out to allow for testing.
func (m *Migration) setTasks(migrationTasks []Task) error {
	m.blockUntil = 0
	ids := make(map[int]bool)
	lastId := 0

	for _, migration := range migrationTasks {
		// migrations must be in ascending order
		if lastId > migration.MigrationId {
			return UnorderedIDErr
		}
		lastId = migration.MigrationId

		// Prevent duplicate IDs
		if ids[migration.MigrationId] {
			return DuplicateIDErr
		}
		ids[migration.MigrationId] = true

		// Make sure blockUntil is set to the last blocking migration
		if migration.DBUnavailable {
			m.blockUntil = migration.MigrationId
		}
	}

	m.tasks = migrationTasks

	return nil
}

// MakeMigration initializes
func MakeMigration(migrationTasks []Task) (*Migration, error) {
	m := &Migration{
		tasks: migrationTasks,
		state: State{
			Err:      nil,
			Status:   StatusPending,
			Running:  false,
			Blocking: true,
		},
	}

	err := m.setTasks(migrationTasks)
	return m, err
}

// GetStatus returns the current status of the migration. This function is thread safe.
func (m *Migration) GetStatus() State {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return State{
		Err:      m.state.Err,
		Status:   m.state.Status,
		Running:  m.state.Running,
		Blocking: m.state.Blocking,
	}
}

// update is a helper to set values in a thread safe way.
func (m *Migration) update(err error, status string, running bool, blocking bool) {
	m.mutex.Lock()

	defer m.mutex.Unlock()

	if err != m.state.Err {
		m.state.Err = err
	}

	if status != m.state.Status {
		m.state.Status = status
	}

	if running != m.state.Running {
		m.state.Running = running
	}

	if blocking != m.state.Blocking {
		m.state.Blocking = blocking
	}
}

// RunMigrations runs all tasks which have been loaded into the migration. It will update the status accordingly as the
// migration runs. This call will block execution until it completes and should be run in a go routine if that is not
// expected.
func (m *Migration) RunMigrations() {
	blocking := true
	for _, task := range m.tasks {
		if task.MigrationId > m.blockUntil {
			blocking = false
		}

		m.update(nil, StatusActivePrefix+task.Description, true, blocking)
		err := task.Handler()

		if err != nil {
			err := fmt.Errorf("%s%d (%s): %v", StatusErrorPrefix, task.MigrationId, task.Description, err)
			// If a migration failed, mark that the migration is blocking and terminate.
			blocking = true
			m.update(err, err.Error(), false, blocking)
			return
		}
	}

	m.update(nil, StatusComplete, false, false)
	return
}
