package migration

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var MigrationError = errors.New("Migration Error")
var SlowSuccessTask1 = MakeTask(1 * time.Second, nil, 1, "slow success")
var SlowSuccessTask2 = MakeTask(1 * time.Second, nil, 2, "slow success")
var SlowSuccessTask3 = MakeTask(1 * time.Second, nil, 3, "slow success")

var FastSuccessTask1 = MakeTask(0 * time.Second, nil, 1, "fast success")
var FastSuccessTask2 = MakeTask(0 * time.Second, nil, 2, "fast success")
var FastSuccessTask3 = MakeTask(0 * time.Second, nil, 3, "fast success")

var FastErrorTask1 = MakeTask(0 * time.Second, MigrationError, 1, "fast error")
var FastErrorTask2 = MakeTask(0 * time.Second, MigrationError, 2, "fast error")
var FastErrorTask3 = MakeTask(0 * time.Second, MigrationError, 3, "fast error")

var SlowErrorTask1 = MakeTask(1 * time.Second, MigrationError, 1, "slow error")
var SlowErrorTask2 = MakeTask(1 * time.Second, MigrationError, 2, "slow error")
var SlowErrorTask3 = MakeTask(1 * time.Second, MigrationError, 3, "slow error")

type testTask struct {
	id          int
	description string
	duration    time.Duration
	err         error
}

func MakeTask(d time.Duration, err error, id int, description string) testTask {
	return testTask{
		id:          id,
		description: description,
		duration:    d,
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
		MigrationId:    tt.id,
		Handler:        handler,
		Description:    tt.description,
	}

}

func TestSuccessfulMigration(t *testing.T) {
	testcases := []struct {
		name        string
		tasks       []testTask
		startupErr  string
		errors      []string
		statuses    []string
		runTime     time.Duration
	}{
		{
			name:        "Duplicate ID error",
			tasks:       []testTask{
				SlowSuccessTask1,
				SlowSuccessTask1,
			},
			startupErr:  DuplicateIDErr.Error(),
			errors:      []string{},
			statuses:    []string{StatusPending},
			runTime:     1 * time.Second,
		},
		{
			name:        "No handlers exit immediately.",
			tasks:       []testTask{},
			startupErr:  "",
			errors:      []string{},
			statuses:    []string{StatusPending, StatusComplete},
			runTime:     0 * time.Second,
		},
		{
			name:        "One task",
			tasks:       []testTask{SlowSuccessTask1},
			startupErr:  "",
			errors:      []string{},
			statuses:    []string{StatusPending, StatusActivePrefix + SlowSuccessTask1.description, StatusComplete},
			runTime:     1 * time.Second,
		},
		{
			name:        "Two tasks",
			tasks:       []testTask{
				SlowSuccessTask1,
				SlowSuccessTask2,
			},
			startupErr:  "",
			errors:      []string{},
			statuses:    []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessTask1.description,
				StatusActivePrefix + SlowSuccessTask2.description,
				StatusComplete},
			runTime:     2 * time.Second,
		},
		{
			name:        "3 Fast tasks",
			tasks:       []testTask{
				FastSuccessTask1,
				FastSuccessTask2,
				FastSuccessTask3,
			},
			startupErr:  "",
			errors:      []string{},
			statuses:    []string{
				StatusPending,
				StatusActivePrefix + FastSuccessTask1.description,
				StatusActivePrefix + FastSuccessTask2.description,
				StatusActivePrefix + FastSuccessTask3.description,
				StatusComplete},
			runTime:     0 * time.Second,
		},
		{
			name:        "Error right away",
			tasks:       []testTask{
				FastErrorTask1,
				SlowSuccessTask2,
				SlowSuccessTask3,
			},
			startupErr:  "",
			errors:      []string{MigrationError.Error()},
			statuses:    []string{
				StatusPending,
				StatusActivePrefix + FastErrorTask1.description,
				StatusErrorPrefix,
				},
			runTime:     0 * time.Second,
		},
		{
			name:        "Error at the end of non blocking tasks",
			tasks:       []testTask{
				SlowSuccessTask1,
				SlowSuccessTask2,
				SlowErrorTask3,
			},
			startupErr:  "",
			errors:      []string{MigrationError.Error()},
			statuses:    []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessTask1.description,
				StatusActivePrefix + SlowSuccessTask2.description,
				StatusActivePrefix + SlowErrorTask3.description,
				StatusErrorPrefix,
			},
			runTime:     3 * time.Second,
		},
	}

	// Fail test at timeout
	timeout := 10 * time.Second
	// Allow the blocktime to be +/- fuzzyness
	fuzzyness := 250 * time.Millisecond

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			recorder := make([]State, 0)
			tasks := make([]Task, 0)
			migration, _ := MakeMigration(tasks)


			for _, testTask := range testcase.tasks {
				tasks = append(tasks, testTask.Get(migration, &recorder))
			}

			err := migration.setTasks(tasks)
			if testcase.startupErr != "" {
				require.EqualError(t, err, testcase.startupErr)
				return
			} else {
				require.Equal(t, "", testcase.startupErr)
			}

			// Initial Status
			recorder = append(recorder, migration.GetStatus())

			start := time.Now()
			migChan := make(chan struct{})
			go func() {
				migration.Start()
				migChan <- struct{}{}
			}()

			select {
			case <- migChan:
				// Final Status
				recorder = append(recorder, migration.GetStatus())
				fmt.Println("Migration complete, check results...")
			case <- time.After(timeout):
				require.Fail(t, "Migration timeout")
				return
			}

			// Verify expected run duration
			end := time.Now()
			runtime := end.Sub(start)
			if testcase.runTime > runtime + fuzzyness || testcase.runTime < runtime - fuzzyness {
				t.Fatalf("runtime outside nominal duration %s != %s", runtime, testcase.runTime)
			}

			// Check the statuses recorded during migration
			errNum := 0
			statusNum := 0
			running := true
			for _, v := range recorder {
				fmt.Println(v)
				if v.Err != nil {
					assert.Contains(t, v.Err.Error(), testcase.errors[errNum])
					errNum++
				}
				if v.Status != "" {
					assert.Contains(t, v.Status, testcase.statuses[statusNum])
					statusNum++
				}
				running = v.Running
			}

			// When the migration is complete, it better not be Running!
			assert.False(t, running)
		})
	}
}
