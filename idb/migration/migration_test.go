package migration

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errMigration = errors.New("Migration Error")
var slowSuccessTask1 = makeTask(1*time.Second, nil, 1, false, "slow success")
var slowSuccessTask2 = makeTask(1*time.Second, nil, 2, false, "slow success")
var slowSuccessTask3 = makeTask(1*time.Second, nil, 3, false, "slow success")

var fastSuccessTask1 = makeTask(0*time.Second, nil, 1, false, "fast success")
var fastSuccessTask2 = makeTask(0*time.Second, nil, 2, false, "fast success")
var fastSuccessTask3 = makeTask(0*time.Second, nil, 3, false, "fast success")

var fastErrorTask1 = makeTask(0*time.Second, errMigration, 1, false, "fast error")
var fastErrorTask2 = makeTask(0*time.Second, errMigration, 2, false, "fast error")
var fastErrorTask3 = makeTask(0*time.Second, errMigration, 3, false, "fast error")

var slowErrorTask1 = makeTask(1*time.Second, errMigration, 1, false, "slow error")
var slowErrorTask2 = makeTask(1*time.Second, errMigration, 2, false, "slow error")
var slowErrorTask3 = makeTask(1*time.Second, errMigration, 3, false, "slow error")

var slowSuccessBlockingTask1 = makeTask(1*time.Second, nil, 1, true, "blocking slow success")
var slowSuccessBlockingTask2 = makeTask(1*time.Second, nil, 2, true, "blocking slow success")
var slowSuccessBlockingTask3 = makeTask(1*time.Second, nil, 3, true, "blocking slow success")

var fastSuccessBlockingTask1 = makeTask(0*time.Second, nil, 1, true, "blocking fast success")
var fastSuccessBlockingTask2 = makeTask(0*time.Second, nil, 2, true, "blocking fast success")
var fastSuccessBlockingTask3 = makeTask(0*time.Second, nil, 3, true, "blocking fast success")

var fastErrorBlockingTask1 = makeTask(0*time.Second, errMigration, 1, true, "blocking fast error")
var fastErrorBlockingTask2 = makeTask(0*time.Second, errMigration, 2, true, "blocking fast error")
var fastErrorBlockingTask3 = makeTask(0*time.Second, errMigration, 3, true, "blocking fast error")

var slowErrorBlockingTask1 = makeTask(1*time.Second, errMigration, 1, true, "blocking slow error")
var slowErrorBlockingTask2 = makeTask(1*time.Second, errMigration, 2, true, "blocking slow error")
var slowErrorBlockingTask3 = makeTask(1*time.Second, errMigration, 3, true, "blocking slow error")

type testTask struct {
	id          int
	description string
	duration    time.Duration
	blocking    bool
	err         error
}

func makeTask(d time.Duration, err error, id int, blocking bool, description string) testTask {
	return testTask{
		id:          id,
		description: description,
		duration:    d,
		blocking:    blocking,
		err:         err,
	}
}

func (tt testTask) Get(migration *Migration, recorder *[]State) Task {
	handler := func() error {
		*recorder = append(*recorder, migration.GetStatus())

		time.Sleep(tt.duration)
		return tt.err
	}

	return Task{
		MigrationID:   tt.id,
		Handler:       handler,
		Description:   tt.description,
		DBUnavailable: tt.blocking,
	}
}

func TestSuccessfulMigration(t *testing.T) {
	testcases := []struct {
		name       string
		tasks      []testTask
		startupErr string
		errors     []string
		statuses   []string
		blocking   []bool
		runTime    time.Duration
	}{
		{
			name: "Duplicate ID error",
			tasks: []testTask{
				slowSuccessTask1,
				slowSuccessTask1,
			},
			startupErr: ErrDuplicateID.Error(),
			errors:     []string{},
			statuses:   []string{StatusPending},
			blocking:   []bool{true},
			runTime:    1 * time.Second,
		},
		{
			name: "Non-ascending ID error",
			tasks: []testTask{
				slowSuccessTask2,
				slowSuccessTask1,
			},
			startupErr: ErrUnorderedID.Error(),
			errors:     []string{},
			statuses:   []string{StatusPending},
			blocking:   []bool{true},
			runTime:    1 * time.Second,
		},
		{
			name:       "No handlers exit immediately.",
			tasks:      []testTask{},
			startupErr: "",
			errors:     []string{},
			statuses:   []string{StatusPending, StatusComplete},
			blocking:   []bool{true, false},
			runTime:    0 * time.Second,
		},
		{
			name:       "One task",
			tasks:      []testTask{slowSuccessTask1},
			startupErr: "",
			errors:     []string{},
			statuses:   []string{StatusPending, StatusActivePrefix + slowSuccessTask1.description, StatusComplete},
			blocking:   []bool{true, false, false},
			runTime:    1 * time.Second,
		},
		{
			name: "Two tasks",
			tasks: []testTask{
				slowSuccessTask1,
				slowSuccessTask2,
			},
			startupErr: "",
			errors:     []string{},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + slowSuccessTask1.description,
				StatusActivePrefix + slowSuccessTask2.description,
				StatusComplete},
			blocking: []bool{true, false, false, false},
			runTime:  2 * time.Second,
		},
		{
			name: "3 fast tasks",
			tasks: []testTask{
				fastSuccessTask1,
				fastSuccessTask2,
				fastSuccessTask3,
			},
			startupErr: "",
			errors:     []string{},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + fastSuccessTask1.description,
				StatusActivePrefix + fastSuccessTask2.description,
				StatusActivePrefix + fastSuccessTask3.description,
				StatusComplete},
			blocking: []bool{true, false, false, false, false},
			runTime:  0 * time.Second,
		},
		{
			name: "Error right away",
			tasks: []testTask{
				fastErrorTask1,
				slowSuccessTask2,
				slowSuccessTask3,
			},
			startupErr: "",
			errors:     []string{errMigration.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + fastErrorTask1.description,
				StatusErrorPrefix,
			},
			blocking: []bool{true, false, true},
			runTime:  0 * time.Second,
		},
		{
			name: "Error at the end of non blocking tasks",
			tasks: []testTask{
				slowSuccessTask1,
				slowSuccessTask2,
				slowErrorTask3,
			},
			startupErr: "",
			errors:     []string{errMigration.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + slowSuccessTask1.description,
				StatusActivePrefix + slowSuccessTask2.description,
				StatusActivePrefix + slowErrorTask3.description,
				StatusErrorPrefix,
			},
			blocking: []bool{true, false, false, false, true},
			runTime:  3 * time.Second,
		},
		{
			name: "Only the first task is blocking",
			tasks: []testTask{
				slowSuccessBlockingTask1,
				slowSuccessTask2,
				slowSuccessTask3,
			},
			startupErr: "",
			errors:     []string{errMigration.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + slowSuccessBlockingTask1.description,
				StatusActivePrefix + slowSuccessTask2.description,
				StatusActivePrefix + slowSuccessTask3.description,
				StatusComplete,
			},
			blocking: []bool{true, true, false, false, false},
			runTime:  3 * time.Second,
		},
		{
			name: "Only the middle task is blocking",
			tasks: []testTask{
				slowSuccessTask1,
				slowSuccessBlockingTask2,
				slowSuccessTask3,
			},
			startupErr: "",
			errors:     []string{errMigration.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + slowSuccessTask1.description,
				StatusActivePrefix + slowSuccessBlockingTask2.description,
				StatusActivePrefix + slowSuccessTask3.description,
				StatusComplete,
			},
			blocking: []bool{true, true, true, false, false},
			runTime:  3 * time.Second,
		},
		{
			name: "Last task is blocking",
			tasks: []testTask{
				slowSuccessTask1,
				slowSuccessTask2,
				slowSuccessBlockingTask3,
			},
			startupErr: "",
			errors:     []string{errMigration.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + slowSuccessTask1.description,
				StatusActivePrefix + slowSuccessTask2.description,
				StatusActivePrefix + slowSuccessBlockingTask3.description,
				StatusComplete,
			},
			blocking: []bool{true, true, true, true, false},
			runTime:  3 * time.Second,
		},
	}

	// Fail test at timeout
	timeout := 10 * time.Second
	// Allow the blocktime to be +/- fuzzyness
	fuzzyness := 250 * time.Millisecond

	for _, testcase := range testcases {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			recorder := make([]State, 0)
			tasks := make([]Task, 0)
			migration, _ := MakeMigration(tasks, nil)

			for _, testTask := range testcase.tasks {
				tasks = append(tasks, testTask.Get(migration, &recorder))
			}

			err := migration.setTasks(tasks)
			if testcase.startupErr != "" {
				require.EqualError(t, err, testcase.startupErr)
				return
			}
			require.Equal(t, "", testcase.startupErr)

			// Initial Status
			recorder = append(recorder, migration.GetStatus())

			start := time.Now()
			migChan := make(chan struct{})
			go func() {
				migration.runMigrations(make(chan struct{}))
				migChan <- struct{}{}
			}()

			select {
			case <-migChan:
				// Final Status
				recorder = append(recorder, migration.GetStatus())
				fmt.Println("Migration complete, check results...")
			case <-time.After(timeout):
				require.Fail(t, "Migration timeout")
				return
			}

			// Verify expected run duration
			end := time.Now()
			runtime := end.Sub(start)
			if testcase.runTime > runtime+fuzzyness || testcase.runTime < runtime-fuzzyness {
				t.Fatalf("runtime outside nominal duration %s != %s", runtime, testcase.runTime)
			}

			// Check the statuses recorded during migration
			errNum := 0
			running := true
			for idx, v := range recorder {
				fmt.Println(v)
				if v.Err != nil {
					require.Contains(t, v.Err.Error(), testcase.errors[errNum])
					errNum++
				}
				require.Contains(t, v.Status, testcase.statuses[idx])
				require.Equal(t, testcase.blocking[idx], v.Blocking)

				running = v.Running
			}

			// When the migration is complete, it better not be Running!
			require.False(t, running)
		})
	}
}

// TestAvailabilityChannelCloses tests that the migration object closes the availability
// channel when blocking migrations finish.
func TestAvailabilityChannelCloses(t *testing.T) {
	// Migration 2 reads on this channel.
	migrationTwoChannel := make(chan struct{})
	defer func() {
		migrationTwoChannel <- struct{}{}
	}()

	tasks := []Task{
		{
			MigrationID: 1,
			Handler: func() error {
				return nil
			},
			DBUnavailable: true,
		},
		{
			MigrationID: 2,
			Handler: func() error {
				<-migrationTwoChannel
				return nil
			},
		},
	}

	m, err := MakeMigration(tasks, nil)
	require.NoError(t, err)

	availableCh := m.RunMigrations()
	select {
	case _, ok := <-availableCh:
		assert.False(t, ok)
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "channel must be closed")
	}
}

// TestAvailabilityChannelClosesNoMigrations tests that the migration object closes
// the availability channel when last migration, which is blocking, finishes.
func TestAvailabilityChannelClosesNoMigrations(t *testing.T) {
	tasks := []Task{
		{
			MigrationID: 1,
			Handler: func() error {
				return nil
			},
			DBUnavailable: true,
		},
	}

	m, err := MakeMigration(tasks, nil)
	require.NoError(t, err)

	availableCh := m.RunMigrations()
	select {
	case _, ok := <-availableCh:
		assert.False(t, ok)
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "channel must be closed")
	}
}

// TestAvailabilityChannelClosesBlockingMigrationLast tests that the migration object
// closes when there are no migrations.
func TestAvailabilityChannelClosesBlockingMigrationLast(t *testing.T) {
	m, err := MakeMigration([]Task{}, nil)
	require.NoError(t, err)

	availableCh := m.RunMigrations()
	select {
	case _, ok := <-availableCh:
		assert.False(t, ok)
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "channel must be closed")
	}
}

// TestAvailabilityChannelDoesNotCloseEarly tests that the migration object closes the availability
// channel only after all blocking migrations are run.
func TestAvailabilityChannelDoesNotCloseEarly(t *testing.T) {
	migrationTwoChannel := make(chan struct{})
	defer func() {
		migrationTwoChannel <- struct{}{}
	}()

	tasks := []Task{
		{
			MigrationID: 1,
			Handler: func() error {
				return nil
			},
		},
		{
			MigrationID: 2,
			Handler: func() error {
				<-migrationTwoChannel
				return nil
			},
			DBUnavailable: true,
		},
	}

	m, err := MakeMigration(tasks, nil)
	require.NoError(t, err)

	availableCh := m.RunMigrations()
	select {
	case <-availableCh:
		assert.Fail(t, "availability channel closed before migrations finish running")
	case <-time.After(5 * time.Millisecond):
	}
}
