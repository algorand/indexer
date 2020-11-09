package migration

import (
	"errors"
	"fmt"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

// ErrDuplicateID is the error returned when the migration is given duplicate migration IDs.
var ErrDuplicateID = errors.New("duplicate ID detected")
// ErrUnorderedID is the error returned when the migration is given migrations that aren't ordered correctly.
var ErrUnorderedID = errors.New("migration IDs must be in ascending order")

// StatusPending is the migration status before the migration has been started.
const StatusPending = "Migration pending"
// StatusComplete is the migration status after the migration successfully completes.
const StatusComplete = "Migrations Complete"
// StatusActivePrefix is the migration status prefix for the currently running migration.
const StatusActivePrefix = "Active migration: "
// StatusErrorPrefix is the status message prefix when there is an error during the migration.
const StatusErrorPrefix = "error during migration "

// Handler is the function which will be executed to perform the migration for this task.
type Handler func() error

// Task is used to define a migration.
type Task struct {
	// MigrationID is an internal ID that can be used by IndexerDb implementations.
	MigrationID int

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
	log        *log.Logger
	mutex      sync.RWMutex
	tasks      []Task
	blockUntil int
	state      State
}

// Broken out to allow for testing.
func (m *Migration) setTasks(migrationTasks []Task) error {
	m.blockUntil = 0
	ids := make(map[int]bool)
	lastID := 0

	for _, migration := range migrationTasks {
		// migrations must be in ascending order
		if lastID > migration.MigrationID {
			return ErrUnorderedID
		}
		lastID = migration.MigrationID

		// Prevent duplicate IDs
		if ids[migration.MigrationID] {
			return ErrDuplicateID
		}
		ids[migration.MigrationID] = true

		// Make sure blockUntil is set to the last blocking migration
		if migration.DBUnavailable {
			m.blockUntil = migration.MigrationID
		}
	}

	m.tasks = migrationTasks

	return nil
}

// MakeMigration initializes
func MakeMigration(migrationTasks []Task, logger *log.Logger) (*Migration, error) {
	m := &Migration{
		log: logger,
		tasks: migrationTasks,
		state: State{
			Err:      nil,
			Status:   StatusPending,
			Running:  false,
			Blocking: true,
		},
	}

	if m.log == nil {
		m.log = log.New()
		m.log.SetFormatter(&log.JSONFormatter{})
		m.log.SetOutput(os.Stdout)
		m.log.SetLevel(log.TraceLevel)
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
		m.log.Println("Setting status: " + status)
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
	m.log.Printf("Running %d migrations.", len(m.tasks))
	blocking := true
	for _, task := range m.tasks {
		if task.MigrationID > m.blockUntil {
			blocking = false
		}

		m.update(nil, StatusActivePrefix+task.Description, true, blocking)
		err := task.Handler()

		if err != nil {
			err := fmt.Errorf("%s%d (%s): %v", StatusErrorPrefix, task.MigrationID, task.Description, err)
			m.log.WithError(err).Errorf("Migration failed")
			// If a migration failed, mark that the migration is blocking and terminate.
			blocking = true
			m.update(err, err.Error(), false, blocking)
			return
		}
	}

	m.update(nil, StatusComplete, false, false)
	m.log.Println("Migration finished successfully.")
	return
}
