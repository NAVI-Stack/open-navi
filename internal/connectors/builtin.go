package connectors

// BuiltinSetupDescriptors returns the static setup descriptors for connectors that
// ship with NAVI. Used by the CLI onboarding flow when the gateway is not yet running.
// This data mirrors internal/navi/plugin/builtin.go and must be kept in sync manually.
func BuiltinSetupDescriptors() []SetupDescriptor {
	return []SetupDescriptor{
		{
			Type:        "telegram",
			DisplayName: "Telegram",
			RequiredParams: []SetupParam{
				{Key: "bot_token", Label: "Bot Token", Description: "From @BotFather", Secret: true},
			},
			OptionalParams: []SetupParam{
				{Key: "owner_chat_id", Label: "Your Chat ID", Description: "From @userinfobot"},
			},
			SetupHint: "Get your bot token from @BotFather. Add your numeric chat ID from @userinfobot if you want owner-directed replies.",
		},
		{
			Type:        "slack",
			DisplayName: "Slack",
			RequiredParams: []SetupParam{
				{Key: "bot_token", Label: "Slack Bot Token", Secret: true},
			},
			OptionalParams: []SetupParam{
				{Key: "app_token", Label: "Slack App Token", Secret: true},
			},
			SetupHint: "Create a Slack app and install it to your workspace to get tokens.",
		},
	}
}
