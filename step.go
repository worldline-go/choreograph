package choreograph

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

var (
	// ErrJobIsNotAFunction implies that something which is not a function was passed for a job.
	ErrJobIsNotAFunction = errors.New("job field is not a function")
	// ErrJobContextAsFirstParameter implies that job function does not have single context.Context parameter.
	ErrJobContextAsFirstParameter = errors.New("job must have first parameter of type context.Context")
	// ErrJobErrorOnOutputRequired implies that there should be error output parameter for job.
	ErrJobErrorOnOutputRequired = errors.New("job must have at least one return parameter of type error")
	// ErrJobFuncIsRequired implies that job is required and cannot be nil.
	ErrJobFuncIsRequired = errors.New("job cannot be nil")
	// ErrJobTooManyOutputParameters implies that job returns too many parameters.
	ErrJobTooManyOutputParameters = errors.New("job must have maximum two return parameter")
	// ErrJobErrorAsLastParameterRequired implies that job last output parameter is not an error.
	ErrJobErrorAsLastParameterRequired = errors.New("job's last output parameter must be of type error")
	// ErrJobTooManyInputParameters implies that job function have too many input parameters.
	ErrJobTooManyInputParameters = errors.New("job has too many input parameters, two total parameters allowed")
	// ErrJobContextParameterRequired implies that job function does not have a context.Context as parameter.
	ErrJobContextParameterRequired = errors.New("job must have at least one parameter of type context.Context")

	// ErrPreCheckIsNotAFunction implies that something which is not a function was passed for a pre-check.
	ErrPreCheckIsNotAFunction = errors.New("preCheck field is not a function")
	// ErrPreCheckContextAsFirstParameter implies that there is no context.Context parameter for pre-check.
	ErrPreCheckContextAsFirstParameter = errors.New("preCheck must have first parameter of type context.Context")
	// ErrPreCheckErrorOnOutputRequired implies that the only output parameter from pre-check function is not an error.
	ErrPreCheckErrorOnOutputRequired = errors.New("preCheck must have at least one return parameter of type error")
	// ErrPreCheckFuncIsRequired implies that pre-check is required and cannot be nil.
	ErrPreCheckFuncIsRequired = errors.New("preCheck cannot be nil")
	// ErrPreCheckTooManyOutputParameters implies that pre-check returns too many parameters.
	ErrPreCheckTooManyOutputParameters = errors.New("preCheck must have maximum two return parameter")
	// ErrPreCheckLastParamTypeErrorRequired implies that pre-check last output parameter is not an error.
	ErrPreCheckLastParamTypeErrorRequired = errors.New("preCheck's last output parameter must be of type error")
	// ErrPreCheckTooManyInputParameters implies that pre-check function have too many input parameters.
	ErrPreCheckTooManyInputParameters = errors.New("preCheck has too many input parameters, two total parameters allowed")
	// ErrPreCheckContextParameterRequired implies that pre-check function does not have a context.Context as parameter.
	ErrPreCheckContextParameterRequired = errors.New("preCheck must have at least one parameter of type context.Context")
)

// Step represent single chunk of process which should be run.
type Step struct {
	// Name is used to store returned data from all steps. Name is also used for better logging experience.
	Name string
	// Job is a function which will be run during the step.
	//
	// It is a function which only parameter is of type context.Context which will be provided from a coordinator.
	// For output of this function there can be one or two return parameters but last one must be an error.
	// If for some reason you want to stop execution of any further step, return ErrExecutionCanceled or wrap it.
	// Example of acceptable functions:
	// - func(ctx context.Context) error
	// - func(ctx context.Context) (int, error)
	// - func(ctx context.Context) (string, error)
	// - func(ctx context.Context, input int) (float64, error)
	Job interface{}
	// PreCheck is a function which will be run before Job, it should ensure that Job can be run.
	//
	// It is a function which only parameter is of type context.Context which will be provided from a coordinator.
	// For output of this function there can be one or two return parameters but last one must be an error.
	// If for some reason you want to stop execution of any further step, return ErrExecutionCanceled or wrap it.
	// Example of acceptable functions:
	// - func(ctx context.Context) error
	// - func(ctx context.Context) (int, error)
	// - func(ctx context.Context) (string, error)
	// - func(ctx context.Context, input int) (float64, error)
	PreCheck interface{}
}

func checkStep(step *Step) error {
	const (
		maxInputParametersCount = 2
		maxReturnsCount         = 2
	)

	if step.Job == nil {
		return ErrJobFuncIsRequired
	}

	if step.PreCheck == nil {
		return ErrPreCheckFuncIsRequired
	}

	funcType := reflect.TypeOf(step.Job)
	if funcType.Kind() != reflect.Func {
		return ErrJobIsNotAFunction
	}

	if funcType.NumIn() == 0 {
		return ErrJobContextParameterRequired
	}

	if funcType.NumIn() > maxInputParametersCount {
		return ErrJobTooManyInputParameters
	}

	if funcType.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return ErrJobContextAsFirstParameter
	}

	if funcType.NumOut() == 0 {
		return ErrJobErrorOnOutputRequired
	}

	if funcType.NumOut() > maxReturnsCount {
		return ErrJobTooManyOutputParameters
	}

	if !funcType.Out(funcType.NumOut() - 1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return ErrJobErrorAsLastParameterRequired
	}

	preCheckType := reflect.TypeOf(step.PreCheck)
	if preCheckType.Kind() != reflect.Func {
		return ErrPreCheckIsNotAFunction
	}

	if preCheckType.NumIn() == 0 {
		return ErrPreCheckContextParameterRequired
	}

	if preCheckType.NumIn() > maxInputParametersCount {
		return ErrPreCheckTooManyInputParameters
	}

	if preCheckType.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return ErrPreCheckContextAsFirstParameter
	}

	if preCheckType.NumOut() == 0 {
		return ErrPreCheckErrorOnOutputRequired
	}

	if preCheckType.NumOut() > maxReturnsCount {
		return ErrPreCheckTooManyOutputParameters
	}

	if !preCheckType.Out(preCheckType.NumOut() - 1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return ErrPreCheckLastParamTypeErrorRequired
	}

	return nil
}

// Steps is a slice of Step.
type Steps []*Step

func (s Steps) StepName(i int) string {
	if i >= len(s) || strings.TrimSpace(s[i].Name) == "" {
		return fmt.Sprintf("#%d", i)
	}

	return s[i].Name
}
