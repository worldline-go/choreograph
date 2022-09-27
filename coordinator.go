// Package choreograph contains sequentially executing processor.
// Such a way that subsequent steps are executed only when the preceding step has succeeded (job finished successfully).
// Each step also has a function that checks if the step should be executed, if so, the work
// specified in the step is executed, otherwise the step is skipped and the next step in the queue is passed.
package choreograph

import (
	"context"
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
	// ErrCoordinatorContextEmpty implies that context passed for NewCoordinator was nil.
	ErrCoordinatorContextEmpty = errors.New("coordinator context cannot be empty")
	// ErrJobFailed implies that job execution failed.
	ErrJobFailed = errors.New("job failed")
	// ErrUnassignableParameter implies that input cannot be used as a parameter in callback function.
	ErrUnassignableParameter = errors.New("cannot assign input to callback")
	// ErrExecutionCanceled implies that execution was stopped intentionally by developer.
	ErrExecutionCanceled = errors.New("execution canceled by callback")
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

	inputs  chan interface{}
	results chan Result
	wg      *sync.WaitGroup

	addStepLock sync.Locker
	err         []error
}

// NewCoordinator creates new executing processor which uses passed context.
// It returns error if context is nil.
func NewCoordinator(ctx context.Context, opts ...Option) (*Coordinator, error) {
	if ctx == nil {
		return nil, ErrCoordinatorContextEmpty
	}

	coordinator := &Coordinator{
		workerCount: runtime.NumCPU(),
		inputs:      make(chan interface{}, bufferSize),
		results:     make(chan Result, bufferSize),
		wg:          new(sync.WaitGroup),
		addStepLock: new(sync.Mutex),
		workers:     nil,
		err:         nil,
	}

	for _, o := range opts {
		o(coordinator)
	}

	if coordinator.workerCount < 1 {
		coordinator.workerCount = 1
	}

	coordinator.workers = make([]*worker, coordinator.workerCount)
	for workerIdx := 0; workerIdx < coordinator.workerCount; workerIdx++ {
		coordinator.workers[workerIdx] = new(worker)

		go coordinator.workers[workerIdx].StartWorker(ctx, coordinator.inputs, coordinator.results, coordinator.wg)
	}

	return coordinator, nil
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

	for idx := 0; idx < len(c.workers); idx++ {
		c.workers[idx].steps = append(c.workers[idx].steps, s)
	}

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
func (c *Coordinator) Run(input interface{}) ([]error, error) {
	resultsChan := c.RunConcurrent([]interface{}{input})

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
// - context.Canceled
func (c *Coordinator) RunConcurrent(inputs []interface{}) <-chan Result {
	c.wg.Add(len(inputs))

	go func(localInputs []interface{}) {
		for i := range localInputs {
			c.inputs <- localInputs[i]
		}

		close(c.inputs)
	}(inputs)

	go func(waitGroup *sync.WaitGroup) {
		waitGroup.Wait()

		close(c.results)
	}(c.wg)

	return c.results
}

// GetExecutionErrors returns all errors received during the process execution.
// [DEPRECATED] Instead check first return parameter from Run method.
func (c *Coordinator) GetExecutionErrors() []error {
	return c.err
}
