package migration

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

var MigrationError = errors.New("Migration Error")
var OneSecondSuccessHandler = MakeHandler(1 * time.Second, nil)
var ErrorHandlerFast = MakeHandler(0 * time.Second, MigrationError)
var ErrorHandlerSlow = MakeHandler(1 * time.Second, MigrationError)

func MakeHandler(d time.Duration, err error) Handler  {
	return func() error {
		time.Sleep(d)
		return err
	}
}

func MakeTask(id int, handler Handler, blocking bool, description string) Task {
	return Task{
		MigrationId:    id,
		Handler:        handler,
		PreventStartup: blocking,
		Description:    description,
	}
}

func TestSuccessfulMigration(t *testing.T) {
	testcases := []struct {
		name        string
		tasks       []Task
		startupErr  string
		errors      []error
		statuses    []string
		blockedTime time.Duration
		runTime     time.Duration
	}{
		{
			name:        "No handlers exit immediately.",
			tasks:       []Task{},
			startupErr:  "",
			errors:      []error{},
			statuses:    []string{StatusInitializing, StatusComplete},
			blockedTime: 0 * time.Second,
			runTime:     0 * time.Second,
		},
		{
			name:        "One second blocking migration handler",
			tasks:       []Task{MakeTask(1, OneSecondSuccessHandler, true, "description")},
			startupErr:  "",
			errors:      []error{},
			statuses:    []string{StatusInitializing, StatusActivePrefix + "description", StatusComplete},
			blockedTime: 1 * time.Second,
			runTime:     1 * time.Second,
		},
		{
			name:        "Duplicate ID error",
			tasks:       []Task{
				MakeTask(1, OneSecondSuccessHandler, true, "blocking"),
				MakeTask(1, OneSecondSuccessHandler, false, "non-blocking"),
			},
			startupErr:  DuplicateIDErr.Error(),
			errors:      []error{},
			statuses:    []string{},
			blockedTime: 1 * time.Second,
			runTime:     1 * time.Second,
		},
		{
			name:        "Different blocking and non blocking",
			tasks:       []Task{
				MakeTask(1, OneSecondSuccessHandler, true, "blocking"),
				MakeTask(2, OneSecondSuccessHandler, false, "non-blocking"),
			},
			startupErr:  "",
			errors:      []error{},
			statuses:    []string{
				StatusInitializing,
				StatusActivePrefix + "blocking",
				StatusActivePrefix + "non-blocking",
				StatusComplete},
			blockedTime: 1 * time.Second,
			runTime:     2 * time.Second,
		},
		{
			name:        "Non-blocking task blocks if followed by a blocking task",
			tasks:       []Task{
				MakeTask(1, OneSecondSuccessHandler, false, "non-blocking"),
				MakeTask(2, OneSecondSuccessHandler, true, "blocking"),
			},
			startupErr:  "",
			errors:      []error{},
			statuses:    []string{
				StatusInitializing,
				StatusActivePrefix + "non-blocking",
				StatusActivePrefix + "blocking",
				StatusComplete},
			blockedTime: 2 * time.Second,
			runTime:     2 * time.Second,
		},
		{
			name:        "Error right away",
			tasks:       []Task{
				MakeTask(1, ErrorHandlerFast, false, "error handler"),
				MakeTask(2, OneSecondSuccessHandler, false, "non-blocking"),
				MakeTask(3, OneSecondSuccessHandler, true, "blocking"),
			},
			startupErr:  "",
			errors:      []error{MigrationError},
			statuses:    []string{
				StatusInitializing,
				StatusActivePrefix + "error handler",
				StatusErrorPrefix,
				},
			blockedTime: 0 * time.Second,
			runTime:     0 * time.Second,
		},
		{
			name:        "Error at the end",
			tasks:       []Task{
				MakeTask(2, OneSecondSuccessHandler, false, "non-blocking"),
				MakeTask(3, OneSecondSuccessHandler, true, "blocking"),
				MakeTask(4, ErrorHandlerSlow, false, "error handler"),
			},
			startupErr:  "",
			errors:      []error{MigrationError},
			statuses:    []string{
				StatusInitializing,
				StatusActivePrefix + "non-blocking",
				StatusActivePrefix + "blocking",
				StatusActivePrefix + "error handler",
				StatusErrorPrefix,
			},
			blockedTime: 2 * time.Second,
			runTime:     3 * time.Second,
		},
	}

	// Fail test at timeout
	timeout := 10 * time.Second
	// Allow the blocktime to be +/- fuzzyness
	fuzzyness := 250 * time.Millisecond

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			errChan := make(chan error)
			statusChan := make(chan string)
			doneChan := make(chan struct{})
			blockChan := make(chan struct{})

			start := time.Now()

			err := RunMigrations(errChan, statusChan, doneChan, blockChan, testcase.tasks...)
			if testcase.startupErr != "" {
				require.EqualError(t, err, testcase.startupErr)
				return
			} else {
				require.Equal(t, "", testcase.startupErr)
			}

			errNum := 0
			statusNum := 0
			blocking := true

			for {
				select {
					case err := <- errChan:
						require.Lessf(t, errNum, len(testcase.errors), "Too many errors returned by RunMigrations. Extra error: %s", err)
						assert.Contains(t, err.Error(), testcase.errors[errNum].Error())
						errNum++

					case status := <- statusChan:
						require.Lessf(t, statusNum, len(testcase.statuses), "Too many statuses returned by RunMigrations. Extra status: ", status)
						require.Contains(t, status, testcase.statuses[statusNum])
						statusNum++

					case <- blockChan:
						blocking = false
						end := time.Now()
						runtime := end.Sub(start)

						if testcase.blockedTime > runtime + fuzzyness || testcase.blockedTime < runtime - fuzzyness {
							t.Fatalf("blocked time outside nominal duration %s != %s", runtime, testcase.blockedTime)
						}

					case <- doneChan:
						end := time.Now()
						runtime := end.Sub(start)

						if testcase.runTime > runtime + fuzzyness || testcase.runTime < runtime - fuzzyness {
							t.Fatalf("runtime outside nominal duration %s != %s", runtime, testcase.runTime)
						}

						// Make sure we received all of the expected events
						assert.Equal(t, len(testcase.statuses), statusNum)
						assert.Equal(t, len(testcase.errors), errNum)
						assert.Equal(t, false, blocking)
						return

					case <-time.After(timeout):
						require.Fail(t, "Migration timeout")
						return
				}
			}
		})
	}
}
