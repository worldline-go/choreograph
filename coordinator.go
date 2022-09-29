// Package choreograph contains sequentially executing processor.
// Such a way that subsequent steps are executed only when the preceding step has succeeded (job finished successfully).
// Each step also has a function that checks if the step should be executed, if so, the work
// specified in the step is executed, otherwise the step is skipped and the next step in the queue is passed.
package choreograph

import (
	"context"
	"reflect"
	"runtime"
	"sync"

	"github.com/pkg/errors"
)

type contextKey string

const DataBagContextKey contextKey = "_coordinator_data_bag"

const (
	bufferSize          = 100
	jobDataPostfix      = "_job"
	preCheckDataPostfix = "_preCheck"
)

var (
	// ErrJobFailed implies that job execution failed.
	ErrJobFailed = errors.New("job failed")
	// ErrUnassignableParameter implies that input cannot be used as a parameter in callback function.
	ErrUnassignableParameter = errors.New("cannot assign input to callback")
	// ErrExecutionCanceled implies that execution was stopped intentionally by developer.
	ErrExecutionCanceled = errors.New("execution canceled by callback")
	// ErrInputMustBeSlice implies that input data set should be a slice of any type.
	ErrInputMustBeSlice = errors.New("input data should be a slice")
)

// Coordinator is an executing processor of defined steps.
//
// Coordinator uses DataBag to store output from all run pre-check and job functions (if more than error was returned).
// To retrieve DataBag you can get it from context using DataBagContextKey, it is always cleared with start of Run.
//
// Use NewCoordinator to create new instance.
// Coordinator implements ProcessExecutioner interface.
type Coordinator struct {
	workerCount int
	workers     []*worker

	inputs      chan interface{}
	inputsLock  sync.Locker
	results     chan Result
	resultsLock sync.Locker
	wg          *sync.WaitGroup

	addStepLock sync.Locker
	steps       Steps
	err         []error
}

// NewCoordinator creates new executing processor which uses passed context.
// It returns error if context is nil.
func NewCoordinator(opts ...Option) *Coordinator {
	coordinator := &Coordinator{
		workerCount: runtime.NumCPU(),
		inputsLock:  new(sync.Mutex),
		resultsLock: new(sync.Mutex),
		addStepLock: new(sync.Mutex),
		wg:          new(sync.WaitGroup),
		steps:       nil,
		inputs:      nil,
		results:     nil,
		workers:     nil,
		err:         nil,
	}

	for _, o := range opts {
		o(coordinator)
	}

	if coordinator.workerCount < 1 {
		coordinator.workerCount = 1
	}

	return coordinator
}

// AddStep adds another step to the queue.
//
// It does a step validation and can return one of following errors:
// - ErrJobIsNotAFunction
// - ErrJobContextAsFirstParameter
// - ErrJobErrorOnOutputRequired
// - ErrJobFuncIsRequired
// - ErrJobTooManyOutputParameters
// - ErrJobErrorAsLastParameterRequired
// - ErrPreCheckIsNotAFunction
// - ErrPreCheckContextAsFirstParameter
// - ErrPreCheckErrorOnOutputRequired
// - ErrPreCheckFuncIsRequired
// - ErrPreCheckTooManyOutputParameters
// - ErrPreCheckLastParamTypeErrorRequired
// Those are step validation errors.
func (c *Coordinator) AddStep(s *Step) error {
	c.addStepLock.Lock()
	defer c.addStepLock.Unlock()

	if err := checkStep(s); err != nil {
		return errors.Wrap(err, "add step")
	}

	c.steps = append(c.steps, s)

	return nil
}

// Run executes the process. Use input to pass immutable data for a run.
//
// Runtime of the process is following, pre-check function is always run before job function,
// if pre-check returns error then job function is skipped and next step is run, in case job returns error
// no further step is being run. First return parameters are execution errors returned by jobs/pre-checks,
// second parameter is runtime error. In case run returns a runtime error you should probably retry the same event.
//
// Possible runtime errors:
// - ErrJobFailed
// - ErrExecutionCanceled
// - context.Canceled
func (c *Coordinator) Run(ctx context.Context, input interface{}) ([]error, error) {
	// as RunConcurrent can return only ErrInputMustBeSlice as error it's safe to skip it
	resultsChan, _ := c.RunConcurrent(ctx, []interface{}{input})

	result := <-resultsChan

	c.err = result.ExecutionErrors

	return result.ExecutionErrors, result.RuntimeError
}

// RunConcurrent executes the process for set of data (same as running Run for each of input).
// This method starts the process in concurrent way, so it will be much faster than regular running.
//
// Runtime of the process is following, pre-check function is always run before job function,
// if pre-check returns error then job function is skipped and next step is run, in case job returns error
// no further step is being run. First return parameters are execution errors returned by jobs/pre-checks,
// second parameter is runtime error. In case run returns a runtime error you should probably retry the same event.
//
// Possible runtime errors:
// - ErrJobFailed
// - ErrExecutionCanceled
// - context.Canceled.
//
// Possible errors:
// - ErrInputMustBeSlice
func (c *Coordinator) RunConcurrent(ctx context.Context, inputs interface{}) (<-chan Result, error) {
	inSlice, ok := toInterfaceSlice(inputs)
	if !ok {
		return c.results, ErrInputMustBeSlice
	}

	c.inputsLock.Lock()
	c.resultsLock.Lock()

	c.init(ctx)

	c.wg.Add(len(inSlice))

	go func(localInputs []interface{}) {
		for i := range localInputs {
			c.inputs <- localInputs[i]
		}

		close(c.inputs)

		c.inputsLock.Unlock()
	}(inSlice)

	go func(waitGroup *sync.WaitGroup) {
		waitGroup.Wait()

		close(c.results)

		c.resultsLock.Unlock()
	}(c.wg)

	return c.results, nil
}

// GetExecutionErrors returns all errors received during the process execution.
// [DEPRECATED] Instead check first return parameter from Run method.
func (c *Coordinator) GetExecutionErrors() []error {
	return c.err
}

func (c *Coordinator) init(ctx context.Context) {
	c.inputs = make(chan interface{}, bufferSize)
	c.results = make(chan Result, bufferSize)
	c.workers = make([]*worker, c.workerCount)

	for workerIdx := 0; workerIdx < c.workerCount; workerIdx++ {
		c.workers[workerIdx] = new(worker)
		c.workers[workerIdx].steps = c.steps

		go c.workers[workerIdx].StartWorker(ctx, c.inputs, c.results, c.wg)
	}
}

func toInterfaceSlice(in interface{}) ([]interface{}, bool) {
	sliceValue := reflect.ValueOf(in)
	if sliceValue.Kind() != reflect.Slice {
		return []interface{}{}, false
	}

	c := sliceValue.Len()

	out := make([]interface{}, c)
	for i := 0; i < c; i++ {
		out[i] = sliceValue.Index(i).Interface()
	}

	return out, true
}
