package cliui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ceoai/navi/internal/connectors"
	"github.com/stretchr/testify/assert"
)

func TestFetchSetupSchema(t *testing.T) {
	schema := []connectors.SetupDescriptor{
		{Type: "telegram", DisplayName: "Telegram"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/connectors/setup-schema", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(schema)
	}))
	defer server.Close()

	got, err := FetchSetupSchema(server.URL, "test-api-key")
	assert.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "telegram", got[0].Type)
}

func TestBuildConnectorParamsForm_RequiredAndOptional(t *testing.T) {
	schema := []connectors.SetupDescriptor{
		{
			Type:        "telegram",
			DisplayName: "Telegram",
			RequiredParams: []connectors.SetupParam{
				{Key: "bot_token", Label: "Bot Token", Secret: true},
			},
			OptionalParams: []connectors.SetupParam{
				{Key: "owner_chat_id", Label: "Your Chat ID"},
			},
		},
	}
	state := NewOnboardingState()
	state.SelectedConnector = "telegram"

	form, fields := BuildConnectorParamsForm(schema, state)
	assert.NotNil(t, form)
	assert.Len(t, fields, 2)
	assert.Equal(t, "bot_token", fields[0].Key)
	assert.Equal(t, "Bot Token", fields[0].Label)
	assert.Equal(t, "owner_chat_id", fields[1].Key)
	assert.Equal(t, "Your Chat ID", fields[1].Label)
}

func TestBuildConnectorParamsForm_SecretField(t *testing.T) {
	schema := []connectors.SetupDescriptor{
		{
			Type:        "slack",
			DisplayName: "Slack",
			RequiredParams: []connectors.SetupParam{
				{Key: "bot_token", Label: "Bot Token", Secret: true},
			},
		},
	}
	state := NewOnboardingState()
	state.SelectedConnector = "slack"

	form, fields := BuildConnectorParamsForm(schema, state)
	assert.NotNil(t, form)
	assert.Len(t, fields, 1)
	assert.Equal(t, "bot_token", fields[0].Key)
	assert.Equal(t, "Bot Token", fields[0].Label)
	// Secret field should be non-nil
	assert.NotNil(t, fields[0].Field)
}

func TestConnectorParamLabelMap(t *testing.T) {
	// Simulate the sync loop in onboarding.go populating ConnectorParamLabels
	schema := []connectors.SetupDescriptor{
		{
			Type:        "telegram",
			DisplayName: "Telegram",
			RequiredParams: []connectors.SetupParam{
				{Key: "bot_token", Label: "Bot Token", Secret: true},
				{Key: "owner_chat_id", Label: "Your Chat ID"},
			},
		},
	}
	state := NewOnboardingState()
	state.SelectedConnector = "telegram"

	_, cFields := BuildConnectorParamsForm(schema, state)

	// Simulate the sync that happens in onboarding.go after form.Run()
	for _, cf := range cFields {
		state.ConnectorParams[cf.Key] = cf.Value
		if cf.Label != "" {
			state.ConnectorParamLabels[cf.Key] = cf.Label
		}
	}

	assert.Equal(t, "Bot Token", state.ConnectorParamLabels["bot_token"])
	assert.Equal(t, "Your Chat ID", state.ConnectorParamLabels["owner_chat_id"])
}

func TestBuildConnectorParamsForm_NoneConnector(t *testing.T) {
	state := NewOnboardingState()
	state.SelectedConnector = "none"
	form, fields := BuildConnectorParamsForm(nil, state)
	assert.Nil(t, form)
	assert.Nil(t, fields)
}

func TestConnectorFormGeneration(t *testing.T) {
	schema := []connectors.SetupDescriptor{
		{
			Type:        "telegram",
			DisplayName: "Telegram",
			RequiredParams: []connectors.SetupParam{
				{Key: "token", Label: "Bot Token", Secret: true},
			},
			OptionalParams: []connectors.SetupParam{
				{Key: "id", Label: "Chat ID"},
			},
		},
	}

	state := NewOnboardingState()
	state.SelectedConnector = "telegram"

	t.Run("form_generation", func(t *testing.T) {
		form, fields := BuildConnectorParamsForm(schema, state)
		assert.NotNil(t, form)
		assert.Len(t, fields, 2)
		assert.Equal(t, "token", fields[0].Key)
		assert.Equal(t, "id", fields[1].Key)
	})

	t.Run("connection_default_is_none", func(t *testing.T) {
		s2 := NewOnboardingState()
		ConnectorSelectForm(schema, s2)
		assert.Equal(t, "none", s2.SelectedConnector)
	})
}
