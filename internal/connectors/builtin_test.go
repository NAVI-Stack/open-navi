package connectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinSetupDescriptors_ContainsTelegram(t *testing.T) {
	descs := BuiltinSetupDescriptors()

	var tg *SetupDescriptor
	for i := range descs {
		if descs[i].Type == "telegram" {
			tg = &descs[i]
			break
		}
	}
	require.NotNil(t, tg, "expected a telegram descriptor")
	assert.Equal(t, "Telegram", tg.DisplayName)
	assert.NotEmpty(t, tg.SetupHint)

	keys := make(map[string]bool)
	for _, p := range tg.RequiredParams {
		keys[p.Key] = true
	}
	assert.True(t, keys["bot_token"], "expected bot_token in required params")
	assert.False(t, keys["owner_chat_id"], "owner_chat_id should not be required")

	optionalKeys := make(map[string]bool)
	for _, p := range tg.OptionalParams {
		optionalKeys[p.Key] = true
	}
	assert.True(t, optionalKeys["owner_chat_id"], "expected owner_chat_id in optional params")
}

func TestBuiltinSetupDescriptors_ContainsSlack(t *testing.T) {
	descs := BuiltinSetupDescriptors()

	var sl *SetupDescriptor
	for i := range descs {
		if descs[i].Type == "slack" {
			sl = &descs[i]
			break
		}
	}
	require.NotNil(t, sl, "expected a slack descriptor")
	assert.Equal(t, "Slack", sl.DisplayName)

	keys := make(map[string]bool)
	for _, p := range sl.RequiredParams {
		keys[p.Key] = true
	}
	assert.True(t, keys["bot_token"], "expected bot_token in required params")
}

func TestBuiltinSetupDescriptors_TelegramSecrets(t *testing.T) {
	descs := BuiltinSetupDescriptors()

	var tg *SetupDescriptor
	for i := range descs {
		if descs[i].Type == "telegram" {
			tg = &descs[i]
			break
		}
	}
	require.NotNil(t, tg)

	params := make(map[string]SetupParam)
	for _, p := range tg.RequiredParams {
		params[p.Key] = p
	}
	for _, p := range tg.OptionalParams {
		params[p.Key] = p
	}

	assert.True(t, params["bot_token"].Secret, "bot_token must be secret")
	assert.False(t, params["owner_chat_id"].Secret, "owner_chat_id must not be secret")
}
