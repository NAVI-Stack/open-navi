package examplemessaging

// ConnectorID is the stable registry identity for this connector.
// The plugin manifest and skill transport blocks should reference this value.
const ConnectorID = "navi.connector.example_messaging"

// Config is intentionally narrow. Secrets should be injected by NAVI's
// secret/config layer, not stored in plugin manifests.
type Config struct {
	APIKey  string
	BaseURL string
}

// Connector adapts NAVI skill operations to the provider API.
// It should not write the World Model directly. It returns structured results
// to the invoking skill executor.
type Connector struct {
	client *Client
}

func New(config Config) *Connector {
	return &Connector{client: NewClient(config)}
}

func (c *Connector) SendMessage(input SendMessageInput) (*SendMessageOutput, error) {
	return c.client.SendMessage(input)
}

func (c *Connector) FetchUpdates(input FetchUpdatesInput) (*FetchUpdatesOutput, error) {
	return c.client.FetchUpdates(input)
}
