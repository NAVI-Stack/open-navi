package orchestration

type Coordinator struct {
	Pipeline *Pipeline
}

func NewCoordinator(pipeline *Pipeline) *Coordinator {
	return &Coordinator{Pipeline: pipeline}
}

func (c *Coordinator) Validate() error {
	if c == nil || c.Pipeline == nil {
		return (&Pipeline{}).Validate()
	}
	return c.Pipeline.Validate()
}
