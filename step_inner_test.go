package choreograph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkStep(t *testing.T) {
	type dummyStruct struct{}
	simpleJobOrPreCheckFunc := func(context.Context) error { return nil }

	tests := []struct {
		name    string
		step    *Step
		wantErr error
	}{
		{
			name: "correct step",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: nil,
		},
		{
			name: "two outputs for job",
			step: &Step{
				Job:      func(context.Context) (string, error) { return "", nil },
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: nil,
		},
		{
			name: "two outputs for preCheck",
			step: &Step{
				PreCheck: func(context.Context) (string, error) { return "", nil },
				Job:      simpleJobOrPreCheckFunc,
			},
			wantErr: nil,
		},
		{
			name: "three outputs for job",
			step: &Step{
				Job:      func(context.Context) (int, string, error) { return 0, "", nil },
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobTooManyOutputParameters,
		},
		{
			name: "three outputs for preCheck",
			step: &Step{
				PreCheck: func(context.Context) (int, string, error) { return 0, "", nil },
				Job:      simpleJobOrPreCheckFunc,
			},
			wantErr: ErrPreCheckTooManyOutputParameters,
		},
		{
			name: "three input parameters for preCheck",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: func(context.Context, int, string) error { return nil },
			},
			wantErr: ErrPreCheckTooManyInputParameters,
		},
		{
			name: "three input parameters for job",
			step: &Step{
				Job:      func(context.Context, int, string) error { return nil },
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobTooManyInputParameters,
		},
		{
			name: "job is not a function",
			step: &Step{
				Job:      dummyStruct{},
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobIsNotAFunction,
		},
		{
			name: "preCheck is not a function",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: dummyStruct{},
			},
			wantErr: ErrPreCheckIsNotAFunction,
		},
		{
			name: "job no context as parameter",
			step: &Step{
				Job:      func() error { return nil },
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobContextParameterRequired,
		},
		{
			name: "preCheck no context as parameter",
			step: &Step{
				PreCheck: func() error { return nil },
				Job:      simpleJobOrPreCheckFunc,
			},
			wantErr: ErrPreCheckContextParameterRequired,
		},
		{
			name: "job no error param on output",
			step: &Step{
				Job:      func(_ context.Context) {},
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobErrorOnOutputRequired,
		},
		{
			name: "preCheck no error param on output",
			step: &Step{
				PreCheck: func(_ context.Context) {},
				Job:      simpleJobOrPreCheckFunc,
			},
			wantErr: ErrPreCheckErrorOnOutputRequired,
		},
		{
			name: "job last param not an error",
			step: &Step{
				Job:      func(_ context.Context) (error, int) { return nil, 0 },
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobErrorAsLastParameterRequired,
		},
		{
			name: "preCheck last param not an error",
			step: &Step{
				PreCheck: func(_ context.Context) (error, int) { return nil, 0 },
				Job:      simpleJobOrPreCheckFunc,
			},
			wantErr: ErrPreCheckLastParamTypeErrorRequired,
		},
		{
			name: "preCheck with no parameters",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: func() {},
			},
			wantErr: ErrPreCheckContextParameterRequired,
		},
		{
			name: "preCheck context as second parameter",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: func(int, context.Context) error { return nil },
			},
			wantErr: ErrPreCheckContextAsFirstParameter,
		},
		{
			name: "job context as second parameter",
			step: &Step{
				PreCheck: simpleJobOrPreCheckFunc,
				Job:      func(int, context.Context) error { return nil },
			},
			wantErr: ErrJobContextAsFirstParameter,
		},
		{
			name: "preCheck no error output function",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: func(context.Context) {},
			},
			wantErr: ErrPreCheckErrorOnOutputRequired,
		},
		{
			name: "job is nil",
			step: &Step{
				Job:      nil,
				PreCheck: simpleJobOrPreCheckFunc,
			},
			wantErr: ErrJobFuncIsRequired,
		},
		{
			name: "preCheck is nil",
			step: &Step{
				Job:      simpleJobOrPreCheckFunc,
				PreCheck: nil,
			},
			wantErr: ErrPreCheckFuncIsRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkStep(tt.step)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
