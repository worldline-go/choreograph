package choreograph

import (
	"context"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

var ErrNoDataBagInContext = errors.New("no data bag in context")

// GetDataBagFromCtx allows to retrieve data bag from provided context.
// It will return ErrNoDataBagInContext if there is no data bag in the context.
func GetDataBagFromCtx(ctx context.Context) (*DataBag, error) {
	db, ok := ctx.Value(DataBagContextKey).(*DataBag)
	if !ok {
		return nil, ErrNoDataBagInContext
	}

	return db, nil
}

// DataBag is a structure which stores a data.
type DataBag struct {
	bag sync.Map
}

// GetJobData allows to fetch the job data from bag using step name.
// If ok is false, it means that no data was stored.
func (d *DataBag) GetJobData(name string) (interface{}, bool) {
	return d.getData(name + jobDataPostfix)
}

// GetPreCheckData allows to fetch the job data from bag using step name.
// If ok is false, it means that no data was stored.
func (d *DataBag) GetPreCheckData(name string) (interface{}, bool) {
	return d.getData(name + preCheckDataPostfix)
}

// getData allows to fetch the data from bag.
// If ok is false, it means that no data with specified key was stored.
func (d *DataBag) getData(name string) (interface{}, bool) {
	val, ok := d.bag.Load(d.getKeyName(name))

	return val, ok
}

func (d *DataBag) setData(name string, data interface{}) {
	d.bag.Store(d.getKeyName(name), data)
}

func (d *DataBag) setJobData(name string, data interface{}) {
	d.setData(name+jobDataPostfix, data)
}

func (d *DataBag) setPreCheckData(name string, data interface{}) {
	d.setData(name+preCheckDataPostfix, data)
}

func (*DataBag) getKeyName(name string) string {
	return strings.ToLower(name)
}

func (d *DataBag) clear() {
	d.bag = sync.Map{}
}
