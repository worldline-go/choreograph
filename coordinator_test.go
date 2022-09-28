package choreograph_test

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/worldline-go/choreograph"
)

type contextKey string

type track struct {
	name         string
	callbackName string
}

type timedTrack struct {
	track
	timestamp int64
}

type executionTracker struct {
	expectedTrack []track
	gotTrack      []timedTrack
}

func (e *executionTracker) getExecutionFn(name, callbackName string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		e.registerExecution(name, callbackName)

		return nil
	}
}

func (e *executionTracker) registerExecution(name, callbackName string) {
	e.gotTrack = append(e.gotTrack, timedTrack{
		track: track{
			name:         name,
			callbackName: callbackName,
		},
		timestamp: time.Now().UnixNano(),
	})
}

func (e *executionTracker) assertExpectation(t *testing.T) {
	t.Helper()

	sort.SliceStable(e.gotTrack, func(i, j int) bool {
		return e.gotTrack[i].timestamp < e.gotTrack[j].timestamp
	})

	require.Equalf(t, len(e.expectedTrack), len(e.gotTrack), "run count should be equal to expectedOrder")

	// check the order
	for i := range e.expectedTrack {
		assert.Equalf(t,
			e.expectedTrack[i].name,
			e.gotTrack[i].name,
			"expected different execution name at index %d", i,
		)
		assert.Equalf(t,
			e.expectedTrack[i].callbackName,
			e.gotTrack[i].callbackName,
			"expected different execution callback name at index %d", i,
		)
	}
}

func TestCoordinator_Run(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name:     "steps run without error",
			testFunc: testStepsRunNoErr,
		},
		{
			name:     "sharing data with context",
			testFunc: testSharingDataWithContext,
		},
		{
			name:     "pre-check can modify context data",
			testFunc: testAccessToDataFromPreviousExecutions,
		},
		{
			name:     "execution order is correct",
			testFunc: testExecutionCorrectness,
		},
		{
			name:     "execution is continued on preCheck error and continued on job error",
			testFunc: testExecutionContinueCancel,
		},
		{
			name:     "retrieving preCheck and job returned errors",
			testFunc: testRetrieveErrors,
		},
		{
			name:     "test passing input data",
			testFunc: testPassingInputData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestCoordinator_RunConcurrent(t *testing.T) {
	const (
		inputsCount int64 = 1000
		runCounts   int   = 2
	)

	var (
		inputs          = make([]interface{}, inputsCount)
		preCheckCounter int64
		jobCounter      int64
		resultsCounter  int64
		idx             int64
	)

	for ; idx < inputsCount; idx++ {
		inputs[idx] = idx
	}

	steps := []*choreograph.Step{
		{
			Name: "first",
			PreCheck: func(ctx context.Context) error {
				atomic.AddInt64(&preCheckCounter, 1)

				return nil
			},
			Job: func(ctx context.Context) error {
				atomic.AddInt64(&jobCounter, 1)

				return nil
			},
		},
		{
			Name: "second",
			PreCheck: func(ctx context.Context) error {
				atomic.AddInt64(&preCheckCounter, 1)

				return nil
			},
			Job: func(ctx context.Context) error {
				atomic.AddInt64(&jobCounter, 1)

				return nil
			},
		},
	}

	c, err := prepareCoordinatorWithSteps(steps, choreograph.WithWorkerCount(4))

	require.NoError(t, err)

	// to check if we can run concurrent multiple times we do it in loop
	for run := 0; run < runCounts; run++ {
		preCheckCounter = 0
		jobCounter = 0
		resultsCounter = 0

		resultsChan, err := c.RunConcurrent(context.Background(), inputs)

		require.NoError(t, err)

		for r := range resultsChan {
			resultsCounter++
			require.Lenf(t, r.ExecutionErrors, 0, "no execution errors expected")
			require.NoError(t, r.RuntimeError)
		}

		expectedCallbackCounter := inputsCount * int64(len(steps))

		require.Equalf(t, inputsCount, resultsCounter, "expected %d on results counter, got %d", inputsCount, resultsCounter)
		require.Equalf(t, expectedCallbackCounter, preCheckCounter, "expected %d on pre-check counter, got %d", expectedCallbackCounter, preCheckCounter)
		require.Equalf(t, expectedCallbackCounter, jobCounter, "expected %d on job counter, got %d", expectedCallbackCounter, jobCounter)
	}
}

func testStepsRunNoErr(t *testing.T) {
	et := executionTracker{
		expectedTrack: []track{
			{name: "first", callbackName: "preCheck"},
			{name: "first", callbackName: "job"},
			{name: "second", callbackName: "preCheck"},
			{name: "second", callbackName: "job"},
		},
	}

	steps := []*choreograph.Step{
		{
			Name:     "first",
			Job:      et.getExecutionFn("first", "job"),
			PreCheck: et.getExecutionFn("first", "preCheck"),
		},
		{
			Name:     "second",
			Job:      et.getExecutionFn("second", "job"),
			PreCheck: et.getExecutionFn("second", "preCheck"),
		},
	}

	c, err := prepareCoordinatorWithSteps(steps)

	require.NoError(t, err)

	execErrs, runtimeErr := c.Run(context.Background(), nil)
	require.Lenf(t, execErrs, 0, "no execution errors expected")
	require.NoError(t, runtimeErr)

	et.assertExpectation(t)
}

func testSharingDataWithContext(t *testing.T) {
	type testData struct {
		text  string
		value int
	}

	var (
		td                            = testData{text: "some text", value: 10}
		contextTestingTKey contextKey = "testingT"
		contextDataKey     contextKey = "data"
	)

	jobExecuted := false

	ctx := context.WithValue(context.Background(), contextDataKey, td)
	ctx = context.WithValue(ctx, contextTestingTKey, t)

	c, err := prepareCoordinatorWithSteps(
		[]*choreograph.Step{{
			Name: "first",
			Job: func(ctx context.Context) error {
				localTd, ok := ctx.Value(contextDataKey).(testData)
				if !ok {
					return fmt.Errorf("expected 'testData' struct object")
				}

				testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
				if !ok {
					return fmt.Errorf("expected 'testingT' struct object")
				}

				assert.Equal(testingT, td, localTd)

				jobExecuted = true

				return nil
			},
			PreCheck: func(ctx context.Context) error { return nil },
		}},
	)

	require.NoError(t, err)

	execErrs, runtimeErr := c.Run(ctx, nil)
	require.Lenf(t, execErrs, 0, "no execution errors expected")
	require.NoError(t, runtimeErr)

	assert.Truef(t, jobExecuted, "expected that job will be executed")
}

func testAccessToDataFromPreviousExecutions(t *testing.T) {
	et := executionTracker{
		expectedTrack: []track{
			{name: "first", callbackName: "preCheck"},
			{name: "first", callbackName: "job"},
			{name: "second", callbackName: "preCheck"},
			{name: "second", callbackName: "job"},
			{name: "third", callbackName: "preCheck"},
			{name: "forth", callbackName: "preCheck"},
			{name: "forth", callbackName: "job"},
		},
	}

	var (
		contextTestingTKey      contextKey = "testingT"
		firstPreCheckReturnVal             = 11
		firstPreCheckReturnErr  error      = nil
		firstJobReturnVal                  = 12
		firstJobReturnErr       error      = nil
		secondPreCheckReturnVal            = 21
		secondPreCheckReturnErr error      = nil
		thirdPreCheckReturnVal             = 21
	)

	c, err := prepareCoordinatorWithSteps(
		[]*choreograph.Step{
			{
				Name: "first",
				PreCheck: func(ctx context.Context) (int, error) {
					et.registerExecution("first", "preCheck")

					return firstPreCheckReturnVal, firstPreCheckReturnErr
				},
				Job: func(ctx context.Context) (int, error) {
					et.registerExecution("first", "job")

					testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
					require.Truef(testingT, ok, "expected 'testingT' struct object")

					dataBag, ok := ctx.Value(choreograph.DataBagContextKey).(*choreograph.DataBag)
					require.Truef(testingT, ok, "expected that data bag is there")

					val, ok := dataBag.GetPreCheckData("first")
					require.Truef(testingT, ok, "expected that data from pre-check is there")

					require.Equal(testingT, firstPreCheckReturnVal, val)

					return firstJobReturnVal, firstJobReturnErr
				},
			},
			{
				Name: "second",
				PreCheck: func(ctx context.Context) (int, error) {
					et.registerExecution("second", "preCheck")

					testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
					require.Truef(testingT, ok, "expected 'testingT' struct object")

					dataBag, ok := ctx.Value(choreograph.DataBagContextKey).(*choreograph.DataBag)
					require.Truef(testingT, ok, "expected that data bag is there")

					val, ok := dataBag.GetJobData("first")
					require.Truef(testingT, ok, "expected that data from pre-check is there")

					require.Equal(testingT, firstJobReturnVal, val)

					return secondPreCheckReturnVal, secondPreCheckReturnErr
				},
				Job: func(ctx context.Context) error {
					et.registerExecution("second", "job")

					testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
					require.Truef(testingT, ok, "expected 'testingT' struct object")

					dataBag, ok := ctx.Value(choreograph.DataBagContextKey).(*choreograph.DataBag)
					require.Truef(testingT, ok, "expected that data bag is there")

					pcFirst, ok := dataBag.GetPreCheckData("first")
					require.Truef(testingT, ok, "expected that data from pre-check is there")

					require.Equal(testingT, firstPreCheckReturnVal, pcFirst)

					jobFirst, ok := dataBag.GetJobData("first")
					require.Truef(testingT, ok, "expected that data from pre-check is there")

					require.Equal(testingT, firstJobReturnVal, jobFirst)

					pcSecond, ok := dataBag.GetPreCheckData("second")
					require.Truef(testingT, ok, "expected that data from pre-check is there")

					require.Equal(testingT, secondPreCheckReturnVal, pcSecond)

					return nil
				},
			},
			{
				Name: "third",
				PreCheck: func(ctx context.Context) (int, error) {
					et.registerExecution("third", "preCheck")

					return thirdPreCheckReturnVal, errors.New("dummy error")
				},
				Job: func(ctx context.Context) error {
					// we don't expect this to run
					et.registerExecution("third", "job")

					return nil
				},
			},
			{
				Name: "forth",
				PreCheck: func(ctx context.Context) error {
					et.registerExecution("forth", "preCheck")

					return nil
				},
				Job: func(ctx context.Context) error {
					// we don't expect this to run
					et.registerExecution("forth", "job")

					testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
					require.Truef(testingT, ok, "expected 'testingT' struct object")

					dataBag, err := choreograph.GetDataBagFromCtx(ctx)
					require.NoError(testingT, err)

					data, ok := dataBag.GetPreCheckData("third")
					require.Truef(testingT, ok, "expected that data from pre-check is there")

					require.Equal(testingT, thirdPreCheckReturnVal, data)

					return nil
				},
			},
		},
	)

	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), contextTestingTKey, t)

	execErrs, runtimeErr := c.Run(ctx, nil)

	// 1 because third pre-check is returning it
	require.Lenf(t, execErrs, 1, "1 execution error expected")
	require.NoError(t, runtimeErr)

	et.assertExpectation(t)
}

func testExecutionCorrectness(t *testing.T) {
	et := executionTracker{
		expectedTrack: []track{
			{name: "first", callbackName: "preCheck"},
			{name: "first", callbackName: "job"},
			{name: "second", callbackName: "preCheck"},
			{name: "second", callbackName: "job"},
			{name: "third", callbackName: "preCheck"},
			{name: "third", callbackName: "job"},
			{name: "fourth", callbackName: "preCheck"},
			{name: "fourth", callbackName: "job"},
			{name: "fifth", callbackName: "preCheck"},
			{name: "fifth", callbackName: "job"},
			{name: "sixth", callbackName: "preCheck"},
			{name: "sixth", callbackName: "job"},
			{name: "seventh", callbackName: "preCheck"},
			{name: "seventh", callbackName: "job"},
			{name: "eight", callbackName: "preCheck"},
			{name: "eight", callbackName: "job"},
			{name: "ninth", callbackName: "preCheck"},
			{name: "ninth", callbackName: "job"},
			{name: "tenth", callbackName: "preCheck"},
			{name: "tenth", callbackName: "job"},
		},
	}

	steps := []*choreograph.Step{
		{
			Name:     "first",
			Job:      et.getExecutionFn("first", "job"),
			PreCheck: et.getExecutionFn("first", "preCheck"),
		},
		{
			Name:     "second",
			Job:      et.getExecutionFn("second", "job"),
			PreCheck: et.getExecutionFn("second", "preCheck"),
		},
		{
			Name:     "third",
			Job:      et.getExecutionFn("third", "job"),
			PreCheck: et.getExecutionFn("third", "preCheck"),
		},
		{
			Name:     "fourth",
			Job:      et.getExecutionFn("fourth", "job"),
			PreCheck: et.getExecutionFn("fourth", "preCheck"),
		},
		{
			Name:     "fifth",
			Job:      et.getExecutionFn("fifth", "job"),
			PreCheck: et.getExecutionFn("fifth", "preCheck"),
		},
		{
			Name:     "sixth",
			Job:      et.getExecutionFn("sixth", "job"),
			PreCheck: et.getExecutionFn("sixth", "preCheck"),
		},
		{
			Name:     "seventh",
			Job:      et.getExecutionFn("seventh", "job"),
			PreCheck: et.getExecutionFn("seventh", "preCheck"),
		},
		{
			Name:     "eight",
			Job:      et.getExecutionFn("eight", "job"),
			PreCheck: et.getExecutionFn("eight", "preCheck"),
		},
		{
			Name:     "ninth",
			Job:      et.getExecutionFn("ninth", "job"),
			PreCheck: et.getExecutionFn("ninth", "preCheck"),
		},
		{
			Name:     "tenth",
			Job:      et.getExecutionFn("tenth", "job"),
			PreCheck: et.getExecutionFn("tenth", "preCheck"),
		},
	}

	ctx := context.Background()

	c, err := prepareCoordinatorWithSteps(steps)
	require.NoError(t, err)

	execErrs, runtimeErr := c.Run(ctx, nil)
	if assert.Lenf(t, execErrs, 0, "no execution errors expected") && assert.NoError(t, runtimeErr) {
		et.assertExpectation(t)
	}
}

func testExecutionContinueCancel(t *testing.T) {
	dummyErr := errors.New("dummy error")
	et := &executionTracker{}

	errReturningFn := func(name, callbackName string) func(_ context.Context) error {
		return func(_ context.Context) error {
			et.registerExecution(name, callbackName)
			return dummyErr
		}
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	testCases := []struct {
		name                        string
		steps                       []*choreograph.Step
		ctx                         context.Context
		expectedTrack               []track
		expectedExecutionErrorCount int
		expectedRuntimeError        error
	}{
		{
			name: "first preCheck error",
			steps: []*choreograph.Step{
				{
					Name:     "first",
					Job:      et.getExecutionFn("first", "job"),
					PreCheck: errReturningFn("first", "preCheck"),
				},
				{
					Name:     "second",
					Job:      et.getExecutionFn("second", "job"),
					PreCheck: et.getExecutionFn("second", "preCheck"),
				},
			},
			expectedTrack: []track{
				{name: "first", callbackName: "preCheck"},
				{name: "second", callbackName: "preCheck"},
				{name: "second", callbackName: "job"},
			},
			expectedExecutionErrorCount: 1,
		},
		{
			name: "first job error",
			steps: []*choreograph.Step{
				{
					Name:     "first",
					PreCheck: et.getExecutionFn("first", "preCheck"),
					Job:      errReturningFn("first", "job"),
				},
				{
					Name:     "second",
					Job:      et.getExecutionFn("second", "job"),
					PreCheck: et.getExecutionFn("second", "preCheck"),
				},
			},
			expectedTrack: []track{
				{name: "first", callbackName: "preCheck"},
				{name: "first", callbackName: "job"},
			},
			expectedRuntimeError:        choreograph.ErrJobFailed,
			expectedExecutionErrorCount: 1,
		},
		{
			name: "second preCheck error",
			steps: []*choreograph.Step{
				{
					Name:     "first",
					PreCheck: et.getExecutionFn("first", "preCheck"),
					Job:      et.getExecutionFn("first", "job"),
				},
				{
					Name:     "second",
					Job:      et.getExecutionFn("second", "job"),
					PreCheck: errReturningFn("second", "preCheck"),
				},
			},
			expectedTrack: []track{
				{name: "first", callbackName: "preCheck"},
				{name: "first", callbackName: "job"},
				{name: "second", callbackName: "preCheck"},
			},
			expectedRuntimeError:        nil,
			expectedExecutionErrorCount: 1,
		},
		{
			name: "second job error",
			steps: []*choreograph.Step{
				{
					Name:     "first",
					PreCheck: et.getExecutionFn("first", "preCheck"),
					Job:      et.getExecutionFn("first", "job"),
				},
				{
					Name:     "second",
					Job:      errReturningFn("second", "job"),
					PreCheck: et.getExecutionFn("second", "preCheck"),
				},
			},
			expectedTrack: []track{
				{name: "first", callbackName: "preCheck"},
				{name: "first", callbackName: "job"},
				{name: "second", callbackName: "preCheck"},
				{name: "second", callbackName: "job"},
			},
			expectedRuntimeError:        choreograph.ErrJobFailed,
			expectedExecutionErrorCount: 1,
		},
		{
			name: "context cancelled",
			steps: []*choreograph.Step{
				{
					Name:     "first",
					PreCheck: et.getExecutionFn("first", "preCheck"),
					Job:      et.getExecutionFn("first", "job"),
				},
				{
					Name:     "second",
					Job:      errReturningFn("second", "job"),
					PreCheck: et.getExecutionFn("second", "preCheck"),
				},
			},
			ctx:                         cancelledCtx,
			expectedTrack:               []track{},
			expectedRuntimeError:        context.Canceled,
			expectedExecutionErrorCount: 0,
		},
		{
			name: "cancel after first pre-check",
			steps: []*choreograph.Step{
				{
					Name: "first",
					PreCheck: func(_ context.Context) error {
						et.registerExecution("first", "preCheck")
						return choreograph.ErrExecutionCanceled
					},
					Job: et.getExecutionFn("first", "job"),
				},
				{
					Name:     "second",
					Job:      et.getExecutionFn("second", "job"),
					PreCheck: et.getExecutionFn("second", "preCheck"),
				},
			},
			expectedTrack: []track{
				{name: "first", callbackName: "preCheck"},
			},
			expectedRuntimeError:        choreograph.ErrExecutionCanceled,
			expectedExecutionErrorCount: 1,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			et.expectedTrack = tt.expectedTrack
			et.gotTrack = []timedTrack{}

			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			c, err := prepareCoordinatorWithSteps(tt.steps)
			require.NoError(t, err)

			execErrs, runtimeErr := c.Run(ctx, nil)
			require.Lenf(t, execErrs, tt.expectedExecutionErrorCount, "no execution errors expected")

			if tt.expectedRuntimeError != nil {
				assert.ErrorIs(t, runtimeErr, tt.expectedRuntimeError)
			} else {
				assert.NoError(t, runtimeErr)
			}

			et.assertExpectation(t)
		})
	}
}

func testRetrieveErrors(t *testing.T) {
	dummyErr := errors.New("dummy error")
	dummyErr2 := errors.New("other dummy error")

	testCases := []struct {
		name             string
		steps            []*choreograph.Step
		expectedErrors   []error
		expectedRunError bool
	}{
		{
			name: "receiving job error",
			steps: []*choreograph.Step{
				{
					Job:      func(_ context.Context) error { return dummyErr },
					PreCheck: func(_ context.Context) error { return nil },
				},
			},
			expectedErrors: []error{
				dummyErr,
			},
			expectedRunError: true,
		},
		{
			name: "second job error",
			steps: []*choreograph.Step{
				{
					Job:      func(_ context.Context) error { return nil },
					PreCheck: func(_ context.Context) error { return nil },
				},
				{
					Job:      func(_ context.Context) error { return dummyErr },
					PreCheck: func(_ context.Context) error { return nil },
				},
			},
			expectedErrors: []error{
				dummyErr,
			},
			expectedRunError: true,
		},
		{
			name: "two preCheck errors",
			steps: []*choreograph.Step{
				{
					Job:      func(_ context.Context) error { return nil },
					PreCheck: func(_ context.Context) error { return dummyErr },
				},
				{
					Job:      func(_ context.Context) error { return nil },
					PreCheck: func(_ context.Context) error { return dummyErr2 },
				},
			},
			expectedErrors: []error{
				dummyErr,
				dummyErr2,
			},
			expectedRunError: false,
		},
		{
			name: "first preCheck error second job error",
			steps: []*choreograph.Step{
				{
					Job:      func(_ context.Context) error { return nil },
					PreCheck: func(_ context.Context) error { return dummyErr },
				},
				{
					Job:      func(_ context.Context) error { return dummyErr2 },
					PreCheck: func(_ context.Context) error { return nil },
				},
			},
			expectedErrors: []error{
				dummyErr,
				dummyErr2,
			},
			expectedRunError: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			c, err := prepareCoordinatorWithSteps(tt.steps)
			require.NoError(t, err)

			execErrs, runtimeErr := c.Run(ctx, nil)

			if tt.expectedRunError {
				require.Error(t, runtimeErr)
			} else {
				require.NoError(t, runtimeErr)
			}

			// this assertion should be removed after next removing GetExecutionErrors method
			require.Equal(t, execErrs, c.GetExecutionErrors())

			if assert.Len(t, execErrs, len(tt.expectedErrors)) {
				for errIdx := range tt.expectedErrors {
					errors.Is(execErrs[errIdx], tt.expectedErrors[errIdx])
				}
			}
		})
	}
}

func testPassingInputData(t *testing.T) {
	const contextTestingTKey contextKey = "testingT"

	type testStruct struct {
		val int
	}

	testCases := []struct {
		name        string
		steps       []*choreograph.Step
		input       interface{}
		expectedErr error
	}{
		{
			name: "provide int into interface",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, 666, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, 666, input)

						return nil
					},
				},
			},
			input: 666,
		},
		{
			name: "provide int into int",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input int) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, 666, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input int) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, 666, input)

						return nil
					},
				},
			},
			input: 666,
		},
		{
			name: "provide string into int",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input int) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						testingT.Errorf("this should not be runned")

						return nil
					},
					PreCheck: func(ctx context.Context, input int) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						testingT.Errorf("this should not be runned")

						return nil
					},
				},
			},
			input:       "text",
			expectedErr: choreograph.ErrUnassignableParameter,
		},
		{
			name: "provide struct into struct",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input testStruct) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, testStruct{val: 5}, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input testStruct) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, testStruct{val: 5}, input)

						return nil
					},
				},
			},
			input: testStruct{val: 5},
		},
		{
			name: "provide struct into interface",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, testStruct{val: 5}, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, testStruct{val: 5}, input)

						return nil
					},
				},
			},
			input: testStruct{val: 5},
		},
		{
			name: "provide struct pointer into interface",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, &testStruct{val: 5}, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, &testStruct{val: 5}, input)

						return nil
					},
				},
			},
			input: &testStruct{val: 5},
		},
		{
			name: "provide struct pointer into struct pointer",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input *testStruct) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, &testStruct{val: 5}, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input *testStruct) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Equal(testingT, &testStruct{val: 5}, input)

						return nil
					},
				},
			},
			input: &testStruct{val: 5},
		},
		{
			name: "provide struct pointer into struct",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input testStruct) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						testingT.Errorf("this should not be runned")

						return nil
					},
					PreCheck: func(ctx context.Context, input testStruct) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						testingT.Errorf("this should not be runned")

						return nil
					},
				},
			},
			input:       &testStruct{val: 5},
			expectedErr: choreograph.ErrUnassignableParameter,
		},
		{
			name: "nil into interface",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Nil(testingT, input)

						return nil
					},
					PreCheck: func(ctx context.Context, input interface{}) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						assert.Nil(testingT, input)

						return nil
					},
				},
			},
			input: nil,
		},
		{
			name: "int and callbacks without additional param",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context) error {
						return nil
					},
					PreCheck: func(ctx context.Context) error {
						return nil
					},
				},
			},
			input: 5,
		},
		{
			name: "nil into int",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, i int) error {
						return nil
					},
					PreCheck: func(ctx context.Context, i int) error {
						return nil
					},
				},
			},
			input: nil,
		},
		{
			name: "provide string into int for job",
			steps: []*choreograph.Step{
				{
					Job: func(ctx context.Context, input int) error {
						testingT, ok := ctx.Value(contextTestingTKey).(*testing.T)
						require.Truef(testingT, ok, "expected 'testingT' struct object")

						testingT.Errorf("this should not be runned")

						return nil
					},
					PreCheck: func(ctx context.Context, input string) error {
						return nil
					},
				},
			},
			input:       "text",
			expectedErr: choreograph.ErrUnassignableParameter,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), contextTestingTKey, t)

			c, err := prepareCoordinatorWithSteps(tt.steps)
			require.NoError(t, err)

			execErrs, runtimeErr := c.Run(ctx, tt.input)
			require.Lenf(t, execErrs, 0, "no execution errors expected")

			if tt.expectedErr != nil {
				assert.ErrorIs(t, runtimeErr, tt.expectedErr)
			} else {
				assert.NoError(t, runtimeErr)
			}
		})
	}
}

func prepareCoordinatorWithSteps(steps []*choreograph.Step, options ...choreograph.Option) (*choreograph.Coordinator, error) {
	c, err := choreograph.NewCoordinator(options...)
	if err != nil {
		return nil, err
	}

	for _, s := range steps {
		if err := c.AddStep(s); err != nil {
			return nil, err
		}
	}

	return c, nil
}
