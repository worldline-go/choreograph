package choreograph

import (
	"context"
	"reflect"
	"sync"

	"github.com/pkg/errors"
)

// ErrUnexpectedType indicates receiving unexpected type when casting.
var ErrUnexpectedType = errors.New("received unexpected type")

// Result is an output for a single piece of work.
type Result struct {
	// Input contains what was provided as an input parameter.
	Input interface{}
	// RuntimeError contains an error that could stop processing the data.
	RuntimeError error
	// ExecutionErrors contains any information/error which occurred during processing the data.
	ExecutionErrors []error
}

type worker struct {
	steps Steps
	err   []error
}

// StartWorker starts listening process to handle new inputs.
func (w *worker) StartWorker(ctx context.Context, inputs <-chan interface{}, results chan<- Result, group *sync.WaitGroup) {
	workerCtx := context.WithValue(ctx, DataBagContextKey, new(DataBag))

	for i := range inputs {
		results <- Result{
			Input:           i,
			RuntimeError:    w.run(workerCtx, i),
			ExecutionErrors: w.err,
		}

		group.Done()
	}
}

func (w *worker) run(ctx context.Context, input interface{}) error {
	w.err = []error{}

	if db, ok := ctx.Value(DataBagContextKey).(*DataBag); ok {
		db.clear()
	}

	for idx := range w.steps {
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "execution stopped before step '%s'", w.steps.StepName(idx))
		default:
			err := w.executeStep(ctx, idx, input)
			if err != nil {
				return errors.Wrapf(err, "step '%s' execution failed", w.steps.StepName(idx))
			}
		}
	}

	return nil
}

func (w *worker) executeStep(ctx context.Context, stepIdx int, input interface{}) error {
	preCheckValue := reflect.ValueOf(w.steps[stepIdx].PreCheck)
	preCheckType := reflect.TypeOf(w.steps[stepIdx].PreCheck)

	preCheckParams, err := w.getCallParams(ctx, preCheckType, input)
	if err != nil {
		return errors.Wrap(err, "preparing preCheck parameters")
	}

	preCheckResp := preCheckValue.Call(preCheckParams)

	if output, ok := isReturnOutput(preCheckResp); ok {
		if db, ok := ctx.Value(DataBagContextKey).(*DataBag); ok {
			db.setPreCheckData(w.steps.StepName(stepIdx), output)
		}
	}

	err = isReturnError(preCheckResp)
	if err != nil {
		w.err = append(w.err, errors.Wrapf(err, "preCheck '%s'", w.steps.StepName(stepIdx)))

		if errors.Is(err, ErrExecutionCanceled) || errors.Is(err, ErrUnexpectedType) {
			return errors.Wrap(err, "preCheck cancelled")
		}

		return nil
	}

	jobValue := reflect.ValueOf(w.steps[stepIdx].Job)
	jobType := reflect.TypeOf(w.steps[stepIdx].Job)

	jobParams, err := w.getCallParams(ctx, jobType, input)
	if err != nil {
		return errors.Wrap(err, "preparing job parameters")
	}

	jobResp := jobValue.Call(jobParams)

	err = isReturnError(jobResp)
	if err != nil {
		w.err = append(w.err, errors.Wrapf(err, "job '%s'", w.steps.StepName(stepIdx)))

		return ErrJobFailed
	}

	if output, ok := isReturnOutput(jobResp); ok {
		if db, ok := ctx.Value(DataBagContextKey).(*DataBag); ok {
			db.setJobData(w.steps.StepName(stepIdx), output)
		}
	}

	return nil
}

func (w *worker) getCallParams(ctx context.Context, callback reflect.Type, input interface{}) ([]reflect.Value, error) {
	inputType := reflect.TypeOf(input)
	inputValue := reflect.ValueOf(input)

	params := make([]reflect.Value, 0)
	params = append(params, reflect.ValueOf(ctx))

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

func isReturnOutput(result []reflect.Value) (interface{}, bool) {
	if len(result) > 1 && result[0].CanInterface() {
		return result[0].Interface(), true
	}

	return nil, false
}

func isReturnError(result []reflect.Value) error {
	if len(result) > 0 && !result[len(result)-1].IsNil() {
		err, ok := result[len(result)-1].Interface().(error)

		if !ok {
			return errors.Wrapf(ErrUnexpectedType, "got '%s' and expected 'error'", result[len(result)-1].Type())
		}

		return err
	}

	return nil
}
