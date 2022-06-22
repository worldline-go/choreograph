package choreograph

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	NoErrorWanted = func(assert.TestingT, error, ...interface{}) bool { return true }
	ErrorWanted   = func(_ assert.TestingT, err error, _ ...interface{}) bool { return err != nil }
)

type (
	storeGetArgs struct {
		keyName  string
		storeVal interface{}
	}
	storeGetTestCase struct {
		name         string
		args         storeGetArgs
		getKeyName   string
		expected     interface{}
		expectExists bool
	}
)

func TestDataBag_GetJobData(t *testing.T) {
	tests := getStoreGetTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := new(DataBag)

			d.setJobData(tt.args.keyName, tt.args.storeVal)

			got, exists := d.GetJobData(tt.getKeyName)

			assert.Equalf(t, tt.expected, got, "GetData(%v)", tt.args.keyName)
			assert.Equalf(t, tt.expectExists, exists, "GetData(%v)", tt.args.keyName)
		})
	}
}

func TestDataBag_GetPreCheckData(t *testing.T) {
	tests := getStoreGetTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := new(DataBag)

			d.setPreCheckData(tt.args.keyName, tt.args.storeVal)

			got, exists := d.GetPreCheckData(tt.getKeyName)

			assert.Equalf(t, tt.expected, got, "GetData(%v)", tt.args.keyName)
			assert.Equalf(t, tt.expectExists, exists, "GetData(%v)", tt.args.keyName)
		})
	}
}

func TestDataBag_clear(t *testing.T) {
	const (
		keyName = "test"
		value   = 1
	)

	d := new(DataBag)

	doStoreCheck := func() {
		// Setting data and getting it (check if was stored)
		d.setData(keyName, value)

		val, ok := d.getData(keyName)

		require.Truef(t, ok, "expected that '"+keyName+"' key will exist")
		require.Equal(t, value, val)
	}

	doStoreCheck()

	// Clearing the bag
	d.clear()

	// Again trying to get the data and check if there is none
	val2, ok2 := d.getData(keyName)

	require.Falsef(t, ok2, "expected that '"+keyName+"' won't exist")
	require.Nil(t, val2)

	// Let's do store one more time to see if working fine
	doStoreCheck()
}

func TestDataBag_getKeyName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "first",
			want: "first",
		},
		{
			name: "Second",
			want: "second",
		},
		{
			name: "ThIrd",
			want: "third",
		},
		{
			name: "Forth!2313-=+",
			want: "forth!2313-=+",
		},
	}

	bag := new(DataBag)

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equalf(t, tt.want, bag.getKeyName(tt.name), "getKeyName(%v)", tt.name)
		})
	}
}

func TestGetDataBagFromCtx(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		args    args
		want    *DataBag
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "data bag in context",
			args: args{
				ctx: context.WithValue(context.Background(), DataBagContextKey, &DataBag{}),
			},
			want:    &DataBag{},
			wantErr: NoErrorWanted,
		},
		{
			name: "no data bag in context",
			args: args{
				ctx: context.Background(),
			},
			want:    nil,
			wantErr: ErrorWanted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDataBagFromCtx(tt.args.ctx)
			if !tt.wantErr(t, err, fmt.Sprintf("GetDataBagFromCtx(%v)", tt.args.ctx)) {
				return
			}

			assert.Equalf(t, tt.want, got, "GetDataBagFromCtx(%v)", tt.args.ctx)
		})
	}
}

func getStoreGetTestCases() []storeGetTestCase {
	return []storeGetTestCase{
		{
			name: "store simple data",
			args: storeGetArgs{
				keyName:  "one",
				storeVal: 1,
			},
			getKeyName:   "one",
			expected:     1,
			expectExists: true,
		},
		{
			name:         "store nothing",
			getKeyName:   "one",
			expected:     nil,
			expectExists: false,
		},
	}
}
