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

type State struct {
	Err      error
	Status   string
	Running  bool
	Blocking bool
}

type Migration struct {
	mutex      sync.Mutex
	state      State
	tasks      []Task
	blockUntil int
}

// Broken out to allow for testing.
func (m *Migration) setTasks(migrationTasks []Task) error {
	m.blockUntil = 0
	set := make(map[int]bool)
	lastId := 0

	for _, migration := range migrationTasks {
		// migrations must be in ascending order
		if lastId > migration.MigrationId {
			return UnorderedIDErr
		}
		lastId = migration.MigrationId

		// Prevent duplicate IDs
		if set[migration.MigrationId] {
			return DuplicateIDErr
		}
		set[migration.MigrationId] = true

		// Make sure blockUntil is set to the last blocking migration
		if migration.DBUnavailable {
			m.blockUntil = migration.MigrationId
		}
	}

	m.tasks = migrationTasks

	return nil
}

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

func (m *Migration) GetStatus() State {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return State{
		Err:      m.state.Err,
		Status:   m.state.Status,
		Running:  m.state.Running,
		Blocking: m.state.Blocking,
	}
}

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

func (m *Migration) Start() {
	blocking := true
	for _, task := range m.tasks {
		if task.MigrationId > m.blockUntil {
			blocking = false
		}

		m.update(nil, StatusActivePrefix+task.Description, true, blocking)
		err := task.Handler()

		if err != nil {
			err := fmt.Errorf("%s%d (%s): %v", StatusErrorPrefix, task.MigrationId, task.Description, err)
			// TODO: If a migration failed, should we block queries?
			blocking = true
			m.update(err, err.Error(), false, blocking)
			return
		}
	}

	m.update(nil, StatusComplete, false, false)
	return
}
