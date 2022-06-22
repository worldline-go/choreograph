package choreograph

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_AddStep(t *testing.T) {
	simpleJobOrPreCheckFunc := func(ctx context.Context) error { return nil }

	tests := []struct {
		name    string
		steps   []*Step
		wantErr bool
	}{
		{
			name: "add one step",
			steps: []*Step{
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
			steps: []*Step{
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
			steps: []*Step{
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
	testCtx, testCancelCtx := context.WithCancel(context.Background())
	defer testCancelCtx()

	type args struct {
		ctx context.Context
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
				ctx: testCtx,
			},
		},
		{
			name:    "missing context",
			args:    args{ctx: nil},
			wantErr: ErrCoordinatorContextEmpty,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCoordinator(tt.args.ctx)

			if tt.wantErr != nil {
				assert.Nil(t, got)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				cancelReflect := reflect.ValueOf(got.ctxCancel)

				assert.NoError(t, err)
				assert.IsType(t, testCtx, got.ctx)
				assert.Truef(t, cancelReflect.Kind() == reflect.Func, "context cancel function should be set")

				// Check if context got DataBag
				db := got.ctx.Value(DataBagContextKey)

				assert.IsType(t, new(DataBag), db)
			}
		})
	}
}
