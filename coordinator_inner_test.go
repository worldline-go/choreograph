package choreograph

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ ProcessExecutioner = (*Coordinator)(nil)

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCoordinator()

			for _, s := range tt.steps {
				err := c.AddStep(s)

				if tt.wantErr {
					require.Error(t, err)
					return
				} else {
					assert.NoError(t, err)
				}
			}

			for wIdx := range c.workers {
				if assert.Len(t, c.workers[wIdx].steps, len(tt.steps), "incorrect length of steps") {
					assert.Equal(t, tt.steps, c.workers[wIdx].steps)
				}
			}
		})
	}
}

func TestNewCoordinator(t *testing.T) {
	type args struct {
		options []Option
	}
	tests := []struct {
		name string
		args args
		want *Coordinator
	}{
		{
			name: "successfully created",
			args: args{},
			want: &Coordinator{
				workerCount: runtime.NumCPU(),
			},
		},
		{
			name: "successfully created with worker count as option",
			args: args{
				options: []Option{WithWorkerCount(10)},
			},
			want: &Coordinator{
				workerCount: 10,
			},
		},
		{
			name: "successfully created with worker count lower than 1",
			args: args{
				options: []Option{WithWorkerCount(0)},
			},
			want: &Coordinator{
				workerCount: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCoordinator(tt.args.options...)

			// Check workers count
			assert.Equalf(t, tt.want.workerCount, got.workerCount, "expected count of workers is '%d', got '%d'", tt.want.workerCount, got.workerCount)
		})
	}
}
