// Package choreograph contains sequentially executing processor.
// Such a way that subsequent steps are executed only when the preceding step has succeeded (job finished successfully).
// Each step also has a function that checks if the step should be executed, if so, the work
// specified in the step is executed, otherwise the step is skipped and the next step in the queue is passed.
package choreograph

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

const DataBagContextKey = "_coordinator_data_bag"

const (
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
	ErrExecutionCanceled = errors.New("execution cancelled by callback")
)

// Coordinator is an executing processor of defined steps.
//
// Coordinator uses DataBag to store output from all run pre-check and job functions (if more than error was returned).
// To retrieve DataBag you can get it from context using DataBagContextKey, it is always cleared with start of Run.
//
// Use NewCoordinator to create new instance.
// Coordinator implements ProcessExecutioner interface.
type Coordinator struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	steps     []*Step
	err       []error
}

// NewCoordinator creates new executing processor which uses passed context.
// It returns error if context is nil.
func NewCoordinator(ctx context.Context) (*Coordinator, error) {
	if ctx == nil {
		return nil, ErrCoordinatorContextEmpty
	}

	ctx = context.WithValue(ctx, DataBagContextKey, new(DataBag))
	ctx, ctxCancel := context.WithCancel(ctx)

	c := Coordinator{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}

	return &c, nil
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
func (c *Coordinator) AddStep(s *Step) error {
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
// no further step is being run.
// In case run returns an error you should probably retry the same event.
// For execution log errors (received from all callbacks) use GetExecutionErrors. This is cleared every run.
//
// Possible errors:
// - ErrJobFailed
// - ErrExecutionCanceled
// - context.Canceled
func (c *Coordinator) Run(input interface{}) error {
	c.err = []error{}

	if db, ok := c.ctx.Value(DataBagContextKey).(*DataBag); ok {
		db.clear()
	}

	for idx := range c.steps {
		select {
		case <-c.ctx.Done():
			return errors.Wrapf(c.ctx.Err(), "execution stopped before step '%s'", c.stepName(idx))
		default:
			err := c.executeStep(idx, input)
			if err != nil {
				return errors.Wrapf(err, "step '%s' execution failed", c.stepName(idx))
			}
		}
	}

	return nil
}

// GetExecutionErrors returns all errors received during the process execution.
func (c *Coordinator) GetExecutionErrors() []error {
	return c.err
}

func (c *Coordinator) executeStep(i int, input interface{}) error {
	preCheckValue := reflect.ValueOf(c.steps[i].PreCheck)
	preCheckType := reflect.TypeOf(c.steps[i].PreCheck)

	preCheckParams, err := c.getCallParams(preCheckType, input)
	if err != nil {
		return errors.Wrap(err, "preparing preCheck parameters")
	}

	preCheckResp := preCheckValue.Call(preCheckParams)

	err = isReturnError(preCheckResp)
	if err != nil {
		c.err = append(c.err, errors.Wrapf(err, "preCheck '%s'", c.stepName(i)))

		if errors.Is(err, ErrExecutionCanceled) {
			c.ctxCancel()
			return ErrExecutionCanceled
		}

		return nil
	}

	if output, ok := isReturnOutput(preCheckResp); ok {
		if db, ok := c.ctx.Value(DataBagContextKey).(*DataBag); ok {
			db.setData(db.getKeyName(c.stepName(i)+preCheckDataPostfix), output)
		}
	}

	jobValue := reflect.ValueOf(c.steps[i].Job)
	jobType := reflect.TypeOf(c.steps[i].Job)

	jobParams, err := c.getCallParams(jobType, input)
	if err != nil {
		return errors.Wrap(err, "preparing job parameters")
	}

	jobResp := jobValue.Call(jobParams)

	err = isReturnError(jobResp)
	if err != nil {
		c.err = append(c.err, errors.Wrapf(err, "job '%s'", c.stepName(i)))

		c.ctxCancel()
		return ErrJobFailed
	}

	if output, ok := isReturnOutput(jobResp); ok {
		if db, ok := c.ctx.Value(DataBagContextKey).(*DataBag); ok {
			db.setData(db.getKeyName(c.stepName(i)+jobDataPostfix), output)
		}
	}

	return nil
}

func (c *Coordinator) getCallParams(callback reflect.Type, input interface{}) ([]reflect.Value, error) {
	inputType := reflect.TypeOf(input)
	inputValue := reflect.ValueOf(input)

	params := make([]reflect.Value, 0)
	params = append(params, reflect.ValueOf(c.ctx))

	if callback.NumIn() > 1 {
		var param reflect.Value

		switch {
		case !inputValue.IsValid():
			param = reflect.New(callback.In(1)).Elem()
		case !inputValue.IsZero():
			if !inputType.AssignableTo(callback.In(1)) {
				return nil, errors.Wrapf(
					ErrUnassignableParameter,
					"cannot assign '%s' to '%s' in preCheck",
					inputType.String(),
					callback.In(1).String(),
				)
			}

			param = inputValue
		}

		params = append(params, param)
	}

	return params, nil
}

func (c *Coordinator) stepName(i int) string {
	if i > len(c.steps) || strings.TrimSpace(c.steps[i].Name) == "" {
		return fmt.Sprintf("#%d", i)
	}

	return c.steps[i].Name
}

func isReturnOutput(result []reflect.Value) (interface{}, bool) {
	if len(result) > 1 && result[0].CanInterface() {
		return result[0].Interface(), true
	}

	return nil, false
}

func isReturnError(result []reflect.Value) error {
	if len(result) > 0 && !result[len(result)-1].IsNil() {
		return result[len(result)-1].Interface().(error)
	}

	return nil
}
