package cliui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/open-navi/navi/internal/connectors"
)

// FetchSetupSchema fetches the connector schema from the gateway.
func FetchSetupSchema(gatewayURL string, apiKey string) ([]connectors.SetupDescriptor, error) {
	req, err := http.NewRequest("GET", gatewayURL+"/api/connectors/setup-schema", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, httpErr := http.DefaultClient.Do(req)
	if httpErr != nil {
		return nil, httpErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch schema: %d", resp.StatusCode)
	}

	var schema []connectors.SetupDescriptor
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// ConnectorSelectForm returns a form for selecting a connector.
func ConnectorSelectForm(schema []connectors.SetupDescriptor, state *OnboardingState) *huh.Form {
	var options []huh.Option[string]
	for _, d := range schema {
		options = append(options, huh.NewOption(d.DisplayName, d.Type))
	}
	options = append(options, huh.NewOption("None / Chat only", "none"))

	if state.SelectedConnector == "" {
		state.SelectedConnector = "none"
	}

	return huh.NewForm(
		huh.NewGroup(
			Select(
				"Add a Connector?",
				"External platforms allow me to reach you remotely.",
				options,
				&state.SelectedConnector,
			),
		),
	)
}

// ConnectorField holds a single dynamic field and its temporary binding value.
type ConnectorField struct {
	Key   string
	Label string
	Value string
	Field huh.Field
}

// BuildConnectorParamsForm generates fields for a connector and returns a form and fields.
func BuildConnectorParamsForm(schema []connectors.SetupDescriptor, state *OnboardingState) (*huh.Form, []ConnectorField) {
	if state.SelectedConnector == "none" || state.SelectedConnector == "" {
		return nil, nil
	}

	var descriptor *connectors.SetupDescriptor
	for i := range schema {
		if schema[i].Type == state.SelectedConnector {
			descriptor = &schema[i]
			break
		}
	}

	if descriptor == nil {
		return nil, nil
	}

	// Build the slice first, then bind Huh fields to slice elements by index.
	// IMPORTANT: the Huh fields capture &cFields[i].Value. We must NOT append to
	// cFields after building the fields or the backing array may be reallocated and
	// those pointers will dangle. Pre-allocate with the exact capacity to be safe.
	total := len(descriptor.RequiredParams) + len(descriptor.OptionalParams)
	cFields := make([]ConnectorField, 0, total)
	for _, p := range descriptor.RequiredParams {
		cFields = append(cFields, ConnectorField{Key: p.Key, Label: p.Label, Value: state.ConnectorParams[p.Key]})
	}
	for _, p := range descriptor.OptionalParams {
		cFields = append(cFields, ConnectorField{Key: p.Key, Label: p.Label, Value: state.ConnectorParams[p.Key]})
	}

	if len(cFields) == 0 {
		return nil, nil
	}

	// Merge param lists in the same order as cFields for index-aligned binding.
	allParams := make([]connectors.SetupParam, 0, total)
	allParams = append(allParams, descriptor.RequiredParams...)
	allParams = append(allParams, descriptor.OptionalParams...)
	reqCount := len(descriptor.RequiredParams)

	// Bind each Huh field to &cFields[i].Value so user input lands in the slice.
	huhFields := make([]huh.Field, len(cFields))
	for i := range cFields {
		p := allParams[i]
		isRequired := i < reqCount
		var field huh.Field
		if p.Secret {
			field = Secret(p.Label, p.Description, p.Placeholder, &cFields[i].Value)
		} else {
			field = Input(p.Label, p.Description, p.Placeholder, &cFields[i].Value)
		}
		if isRequired {
			label := p.Label
			if f, ok := field.(*huh.Input); ok {
				f.Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("%s is required", label)
					}
					return nil
				})
			}
		}
		cFields[i].Field = field
		huhFields[i] = field
	}

	return huh.NewForm(
		huh.NewGroup(huhFields...).
			Title(fmt.Sprintf("%s Settings", descriptor.DisplayName)).
			Description(descriptor.SetupHint),
	), cFields
}
