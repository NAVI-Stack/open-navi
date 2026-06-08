package petbridge

// ConnectorID is the stable registry identity for this connector.
// The plugin manifest and skill transport blocks should reference this value.
const ConnectorID = "navi.connector.pet_bridge"

// Config is intentionally narrow. Secrets should be injected by NAVI's
// secret/config layer, not stored in plugin manifests.
type Config struct {
	BridgeToken string
	BridgeURL   string // WebSocket/IPC endpoint to PET desktop shell
}

// Connector adapts NAVI skill operations to the PET desktop bridge.
// It should not write the World Model directly. It returns structured results
// to the invoking skill executor.
type Connector struct {
	client *Client
}

func New(config Config) *Connector {
	return &Connector{client: NewClient(config)}
}

func (c *Connector) FileRead(input FileReadInput) (*FileReadOutput, error) {
	return c.client.FileRead(input)
}

func (c *Connector) FileWrite(input FileWriteInput) (*FileWriteOutput, error) {
	return c.client.FileWrite(input)
}

func (c *Connector) ListDir(input ListDirInput) (*ListDirOutput, error) {
	return c.client.ListDir(input)
}

func (c *Connector) FolderPick(input FolderPickInput) (*FolderPickOutput, error) {
	return c.client.FolderPick(input)
}
