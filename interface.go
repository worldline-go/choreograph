package choreograph

// ProcessExecutioner describes capabilities of processor which performs
// actions according to a specified scheme for given input data.
type ProcessExecutioner interface {
	// AddStep allows to queue up a step to be executed.
	AddStep(s *Step) error
	// Run starts processing for data passed as input.
	Run(input interface{}) error
	// GetExecutionErrors allows to collect all errors which happen during data processing.
	GetExecutionErrors() []error
}
