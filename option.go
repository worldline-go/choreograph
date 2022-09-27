package choreograph

type Option func(c *Coordinator)

// WithWorkerCount option allows to define how many workers we want to use.
func WithWorkerCount(workerCount int) func(c *Coordinator) {
	return func(c *Coordinator) {
		c.workerCount = workerCount
	}
}
