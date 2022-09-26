package choreograph

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_AddStep(t *testing.T) {
	simpleJobOrPreCheckFunc := func(ctx context.Context) error { return nil }

	tests := []struct {
		name    string
		steps   Steps
		wantErr bool
	}{
		{
			name: "add one step",
			steps: Steps{
				{
					Name:     "first",
					Job:      simpleJobOrPreCheckFunc,
					PreCheck: simpleJobOrPreCheckFunc,
				},
			},
			wantErr: false,
		},
		{
			name: "add several steps",
			steps: Steps{
				{
					Name:     "first",
					Job:      simpleJobOrPreCheckFunc,
					PreCheck: simpleJobOrPreCheckFunc,
				},
				{
					Name:     "second",
					Job:      simpleJobOrPreCheckFunc,
					PreCheck: simpleJobOrPreCheckFunc,
				},
				{
					Name:     "third",
					Job:      simpleJobOrPreCheckFunc,
					PreCheck: simpleJobOrPreCheckFunc,
				},
			},
			wantErr: false,
		},
		{
			name: "error when adding step",
			steps: Steps{
				{
					Name:     "first",
					Job:      simpleJobOrPreCheckFunc,
					PreCheck: func() {},
				},
			},
			wantErr: true,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCoordinator(ctx)
			require.NoError(t, err)

			for _, s := range tt.steps {
				err := c.AddStep(s)

				if tt.wantErr {
					require.Error(t, err)
					return
				} else {
					assert.NoError(t, err)
				}
			}

			if assert.Len(t, c.steps, len(tt.steps), "incorrect length of steps") {
				assert.Equal(t, tt.steps, c.steps)
			}
		})
	}
}

func TestNewCoordinator(t *testing.T) {
	testCtx := context.WithValue(context.Background(), "dummy_key", "dummy_value")

	type args struct {
		ctx     context.Context
		options []Option
	}
	tests := []struct {
		name    string
		args    args
		want    *Coordinator
		wantErr error
	}{
		{
			name: "successfully created",
			args: args{ctx: testCtx},
			want: &Coordinator{
				workerCount: runtime.NumCPU(),
			},
		},
		{
			name:    "missing context",
			args:    args{ctx: nil},
			wantErr: ErrCoordinatorContextEmpty,
		},
		{
			name: "successfully created with worker count as option",
			args: args{
				ctx:     testCtx,
				options: []Option{WithWorkerCount(10)},
			},
			want: &Coordinator{
				workerCount: 10,
			},
		},
		{
			name: "successfully created with worker count lower than 1",
			args: args{
				ctx:     testCtx,
				options: []Option{WithWorkerCount(0)},
			},
			want: &Coordinator{
				workerCount: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCoordinator(tt.args.ctx, tt.args.options...)

			if tt.wantErr != nil {
				assert.Nil(t, got)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)

				// Check workers count
				assert.Equalf(t, tt.want.workerCount, got.workerCount, "expected count of workers is '%d', got '%d'", tt.want.workerCount, got.workerCount)
			}
		})
	}
}
