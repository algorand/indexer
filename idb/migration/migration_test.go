package migration

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var MigrationError = errors.New("Migration Error")
var SlowSuccessTask1 = MakeTask(1*time.Second, nil, 1, false, "slow success")
var SlowSuccessTask2 = MakeTask(1*time.Second, nil, 2, false, "slow success")
var SlowSuccessTask3 = MakeTask(1*time.Second, nil, 3, false, "slow success")

var FastSuccessTask1 = MakeTask(0*time.Second, nil, 1, false, "fast success")
var FastSuccessTask2 = MakeTask(0*time.Second, nil, 2, false, "fast success")
var FastSuccessTask3 = MakeTask(0*time.Second, nil, 3, false, "fast success")

var FastErrorTask1 = MakeTask(0*time.Second, MigrationError, 1, false, "fast error")
var FastErrorTask2 = MakeTask(0*time.Second, MigrationError, 2, false, "fast error")
var FastErrorTask3 = MakeTask(0*time.Second, MigrationError, 3, false, "fast error")

var SlowErrorTask1 = MakeTask(1*time.Second, MigrationError, 1, false, "slow error")
var SlowErrorTask2 = MakeTask(1*time.Second, MigrationError, 2, false, "slow error")
var SlowErrorTask3 = MakeTask(1*time.Second, MigrationError, 3, false, "slow error")

var SlowSuccessBlockingTask1 = MakeTask(1*time.Second, nil, 1, true, "blocking slow success")
var SlowSuccessBlockingTask2 = MakeTask(1*time.Second, nil, 2, true, "blocking slow success")
var SlowSuccessBlockingTask3 = MakeTask(1*time.Second, nil, 3, true, "blocking slow success")

var FastSuccessBlockingTask1 = MakeTask(0*time.Second, nil, 1, true, "blocking fast success")
var FastSuccessBlockingTask2 = MakeTask(0*time.Second, nil, 2, true, "blocking fast success")
var FastSuccessBlockingTask3 = MakeTask(0*time.Second, nil, 3, true, "blocking fast success")

var FastErrorBlockingTask1 = MakeTask(0*time.Second, MigrationError, 1, true, "blocking fast error")
var FastErrorBlockingTask2 = MakeTask(0*time.Second, MigrationError, 2, true, "blocking fast error")
var FastErrorBlockingTask3 = MakeTask(0*time.Second, MigrationError, 3, true, "blocking fast error")

var SlowErrorBlockingTask1 = MakeTask(1*time.Second, MigrationError, 1, true, "blocking slow error")
var SlowErrorBlockingTask2 = MakeTask(1*time.Second, MigrationError, 2, true, "blocking slow error")
var SlowErrorBlockingTask3 = MakeTask(1*time.Second, MigrationError, 3, true, "blocking slow error")

type testTask struct {
	id          int
	description string
	duration    time.Duration
	blocking    bool
	err         error
}

func MakeTask(d time.Duration, err error, id int, blocking bool, description string) testTask {
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
		MigrationId:   tt.id,
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
				SlowSuccessTask1,
				SlowSuccessTask1,
			},
			startupErr: DuplicateIDErr.Error(),
			errors:     []string{},
			statuses:   []string{StatusPending},
			blocking:   []bool{true},
			runTime:    1 * time.Second,
		},
		{
			name: "Non-ascending ID error",
			tasks: []testTask{
				SlowSuccessTask2,
				SlowSuccessTask1,
			},
			startupErr: UnorderedIDErr.Error(),
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
			tasks:      []testTask{SlowSuccessTask1},
			startupErr: "",
			errors:     []string{},
			statuses:   []string{StatusPending, StatusActivePrefix + SlowSuccessTask1.description, StatusComplete},
			blocking:   []bool{true, false, false},
			runTime:    1 * time.Second,
		},
		{
			name: "Two tasks",
			tasks: []testTask{
				SlowSuccessTask1,
				SlowSuccessTask2,
			},
			startupErr: "",
			errors:     []string{},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessTask1.description,
				StatusActivePrefix + SlowSuccessTask2.description,
				StatusComplete},
			blocking: []bool{true, false, false, false},
			runTime:  2 * time.Second,
		},
		{
			name: "3 Fast tasks",
			tasks: []testTask{
				FastSuccessTask1,
				FastSuccessTask2,
				FastSuccessTask3,
			},
			startupErr: "",
			errors:     []string{},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + FastSuccessTask1.description,
				StatusActivePrefix + FastSuccessTask2.description,
				StatusActivePrefix + FastSuccessTask3.description,
				StatusComplete},
			blocking: []bool{true, false, false, false, false},
			runTime:  0 * time.Second,
		},
		{
			name: "Error right away",
			tasks: []testTask{
				FastErrorTask1,
				SlowSuccessTask2,
				SlowSuccessTask3,
			},
			startupErr: "",
			errors:     []string{MigrationError.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + FastErrorTask1.description,
				StatusErrorPrefix,
			},
			blocking: []bool{true, false, true},
			runTime:  0 * time.Second,
		},
		{
			name: "Error at the end of non blocking tasks",
			tasks: []testTask{
				SlowSuccessTask1,
				SlowSuccessTask2,
				SlowErrorTask3,
			},
			startupErr: "",
			errors:     []string{MigrationError.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessTask1.description,
				StatusActivePrefix + SlowSuccessTask2.description,
				StatusActivePrefix + SlowErrorTask3.description,
				StatusErrorPrefix,
			},
			blocking: []bool{true, false, false, false, true},
			runTime:  3 * time.Second,
		},
		{
			name: "Only the first task is blocking",
			tasks: []testTask{
				SlowSuccessBlockingTask1,
				SlowSuccessTask2,
				SlowSuccessTask3,
			},
			startupErr: "",
			errors:     []string{MigrationError.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessBlockingTask1.description,
				StatusActivePrefix + SlowSuccessTask2.description,
				StatusActivePrefix + SlowSuccessTask3.description,
				StatusComplete,
			},
			blocking: []bool{true, true, false, false, false},
			runTime:  3 * time.Second,
		},
		{
			name: "Only the middle task is blocking",
			tasks: []testTask{
				SlowSuccessTask1,
				SlowSuccessBlockingTask2,
				SlowSuccessTask3,
			},
			startupErr: "",
			errors:     []string{MigrationError.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessTask1.description,
				StatusActivePrefix + SlowSuccessBlockingTask2.description,
				StatusActivePrefix + SlowSuccessTask3.description,
				StatusComplete,
			},
			blocking: []bool{true, true, true, false, false},
			runTime:  3 * time.Second,
		},
		{
			name: "Last task is blocking",
			tasks: []testTask{
				SlowSuccessTask1,
				SlowSuccessTask2,
				SlowSuccessBlockingTask3,
			},
			startupErr: "",
			errors:     []string{MigrationError.Error()},
			statuses: []string{
				StatusPending,
				StatusActivePrefix + SlowSuccessTask1.description,
				StatusActivePrefix + SlowSuccessTask2.description,
				StatusActivePrefix + SlowSuccessBlockingTask3.description,
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
				migration.RunMigrations()
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
