package choreograph

import "context"

// ProcessExecutioner describes capabilities of processor which performs
// actions according to a specified scheme for given input data.
type ProcessExecutioner interface {
	// AddStep allows to queue up a step to be executed.
	AddStep(s *Step) error
	// Run starts processing for data passed as input.
	Run(ctx context.Context, input interface{}) ([]error, error)
	// RunConcurrent starts processing for data set passed as inputs.
	// Processing is done in concurrent way.
	RunConcurrent(ctx context.Context, inputs interface{}) (<-chan Result, error)
	// GetExecutionErrors allows to collect all errors which happen during data processing.
	// [DEPRECATED] Instead check first return parameter from Run method.
	GetExecutionErrors() []error
}
