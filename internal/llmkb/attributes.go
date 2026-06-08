package llmkb

type Attribute struct {
	// [Governed] Key stores the attribute key.
	Key string `json:"key" yaml:"key"`
	// [Governed] Value stores the attribute string value.
	Value string `json:"value" yaml:"value"`
}
