package migration

import (
	"errors"
	"fmt"
	"sync"
)

var DuplicateIDErr = errors.New("Duplicate ID detected.")

const StatusPending = "Migration pending"
const StatusComplete = "Migrations Complete"
const StatusActivePrefix = "Active migration: "
const StatusErrorPrefix = "error during migration "

// A migration function should take care of writing back to metastate migration row
type Handler func() error

type Task struct {
	MigrationId int

	Handler Handler

	// Description of the migration
	Description string
}

type State struct {
	Err     error
	Status  string
	Running bool
}

type Migration struct {
	mutex sync.Mutex

	state State
	tasks []Task
}

// Broken out to allow for testing.
func (m *Migration) setTasks(migrationTasks []Task) error {
	set := make(map[int]bool)

	for _, migration := range migrationTasks {
		// Prevent duplicate IDs
		if set[migration.MigrationId] {
			return DuplicateIDErr
		}
		set[migration.MigrationId] = true
	}

	m.tasks = migrationTasks

	return nil
}

func MakeMigration(migrationTasks []Task) (*Migration, error) {
	m := &Migration{
		tasks: migrationTasks,
		state: State{
			Err:     nil,
			Status:  StatusPending,
			Running: false,
		},
	}

	err := m.setTasks(migrationTasks)
	return m, err
}

func (m *Migration) GetStatus() State {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return State{
		Err:     m.state.Err,
		Status:  m.state.Status,
		Running: m.state.Running,
	}
}

func (m *Migration) update(err error, status string, running bool) {
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
}

func (m *Migration) Start() {
	for _, task := range m.tasks {
		m.update(nil, StatusActivePrefix + task.Description, true)
		err := task.Handler()

		if err != nil {
			err := fmt.Errorf("%s%d (%s): %v", StatusErrorPrefix, task.MigrationId, task.Description, err)
			m.update(err, err.Error(), false)
			return
		}
	}

	m.update(nil, StatusComplete, false)
	return
}
