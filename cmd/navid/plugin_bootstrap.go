package main

import (
	"context"
	"strings"

	pkgconn "github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/config"
	intconnectors "github.com/ceoai/navi/internal/connectors"
	"github.com/ceoai/navi/internal/navi"
	documentknowledge "github.com/ceoai/navi/plugins/document-knowledge/handlers"
	_ "github.com/ceoai/navi/plugins/llm-anthropic/providers/anthropic"
	_ "github.com/ceoai/navi/plugins/llm-ollama/providers/ollama"
	_ "github.com/ceoai/navi/plugins/llm-openai/providers/openai"
	navicoder "github.com/ceoai/navi/plugins/navi-coder/handlers"
	navicontacts "github.com/ceoai/navi/plugins/navi-contacts/handlers"
	naviprogrammer "github.com/ceoai/navi/plugins/navi-programmer/handlers"
	navisearch "github.com/ceoai/navi/plugins/navi-search/handlers"
	selfdiagnostic "github.com/ceoai/navi/plugins/self-diagnostic/handlers"
	skillcreator "github.com/ceoai/navi/plugins/skill-creator/handlers"
	"github.com/ceoai/navi/plugins/slack/connectors/slack"
	"github.com/ceoai/navi/plugins/telegram/connectors/telegram"
)

type pluginErrorRecorder func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error

func registerTelegramAccountFactory(connectorRegistry *intconnectors.Registry, acct config.ResolvedTelegramAccount, saveErrorRecord pluginErrorRecorder, intakeEmit ...func(ctx context.Context, data []byte) error) {
	account := acct
	var emitFn func(ctx context.Context, data []byte) error
	if len(intakeEmit) > 0 {
		emitFn = intakeEmit[0]
	}
	connectorRegistry.RegisterFactory(account.ConnectorName, func(cfg *config.Config, b bus.Bus) (pkgconn.Connector, error) {
		return telegram.NewBot(telegram.Config{
			Name:            account.ConnectorName,
			TelegramToken:   account.BotToken,
			GatewayURL:      account.GatewayURL,
			GatewaySecret:   cfg.Gateway.SharedSecret,
			OwnerChatID:     account.OwnerChatID,
			AllowFrom:       account.AllowFrom,
			PairingCode:     account.PairingCode,
			APIURL:          account.APIURL,
			WebhookURL:      account.WebhookURL,
			WebhookSecret:   account.WebhookSecret,
			Groups:          telegramGroupsFromConfig(account.Groups),
			SaveErrorRecord: saveErrorRecord,
			IntakeEmit:      emitFn,
		}), nil
	})
}

// telegramGroupsFromConfig converts config group settings into the connector's own
// group-config shape, keeping the telegram package free of a config dependency.
func telegramGroupsFromConfig(groups map[string]config.TelegramGroupConfig) map[string]telegram.GroupConfig {
	if len(groups) == 0 {
		return nil
	}
	out := make(map[string]telegram.GroupConfig, len(groups))
	for key, g := range groups {
		out[key] = telegram.GroupConfig{
			Enabled:        g.Enabled,
			RequireMention: g.RequireMention,
		}
	}
	return out
}

func registerSlackFactory(connectorRegistry *intconnectors.Registry, saveErrorRecord pluginErrorRecorder) {
	connectorRegistry.RegisterFactory("slack", func(cfg *config.Config, b bus.Bus) (pkgconn.Connector, error) {
		return slack.NewBot(slack.Config{
			SlackBotToken:   cfg.Connectors.Slack.BotToken,
			SlackAppToken:   cfg.Connectors.Slack.AppToken,
			GatewayURL:      cfg.Connectors.Slack.GatewayURL,
			GatewaySecret:   cfg.Gateway.SharedSecret,
			AllowedUserIDs:  cfg.Connectors.Slack.AllowedUserIDs,
			SaveErrorRecord: saveErrorRecord,
		}), nil
	})
}

func registerTelegramSetupFactory(connectorRegistry *intconnectors.Registry, saveErrorRecord pluginErrorRecorder, intakeEmit func(ctx context.Context, data []byte) error) {
	connectorRegistry.RegisterFactory("telegram", func(cfg *config.Config, b bus.Bus) (pkgconn.Connector, error) {
		resolvedAccounts, err := config.ResolveTelegramAccounts(cfg.Connectors.Telegram)
		if err != nil {
			return nil, err
		}
		for _, acct := range resolvedAccounts {
			if acct.ConnectorName != "telegram" {
				continue
			}
			return telegram.NewBot(telegram.Config{
				Name:            acct.ConnectorName,
				TelegramToken:   acct.BotToken,
				GatewayURL:      acct.GatewayURL,
				GatewaySecret:   cfg.Gateway.SharedSecret,
				OwnerChatID:     acct.OwnerChatID,
				AllowFrom:       acct.AllowFrom,
				PairingCode:     acct.PairingCode,
				APIURL:          acct.APIURL,
				WebhookURL:      acct.WebhookURL,
				WebhookSecret:   acct.WebhookSecret,
				Groups:          telegramGroupsFromConfig(acct.Groups),
				SaveErrorRecord: saveErrorRecord,
				IntakeEmit:      intakeEmit,
			}), nil
		}

		return telegram.NewBot(telegram.Config{
			Name:            "telegram",
			TelegramToken:   cfg.Connectors.Telegram.BotToken,
			GatewayURL:      nonEmptyOr(strings.TrimSpace(cfg.Connectors.Telegram.GatewayURL), "http://localhost:6284"),
			GatewaySecret:   cfg.Gateway.SharedSecret,
			OwnerChatID:     cfg.Connectors.Telegram.OwnerChatID,
			AllowFrom:       cfg.Connectors.Telegram.AllowFrom,
			PairingCode:     cfg.Connectors.Telegram.PairingCode,
			APIURL:          nonEmptyOr(strings.TrimSpace(cfg.Connectors.Telegram.APIURL), "https://api.telegram.org"),
			WebhookURL:      cfg.Connectors.Telegram.WebhookURL,
			WebhookSecret:   cfg.Connectors.Telegram.WebhookSecret,
			Groups:          telegramGroupsFromConfig(cfg.Connectors.Telegram.Groups),
			SaveErrorRecord: saveErrorRecord,
			IntakeEmit:      intakeEmit,
		}), nil
	})
}

func registerBuiltinSkillHandlers(ctx navi.SkillHandlerContext) {
	navisearch.RegisterSearchHandler("scout-search", ctx.BraveSearchKey)
	if ctx.Hub != nil {
		navisearch.RegisterHubSearchHandler("scout-search", ctx.Hub)
	}
	if ctx.DB != nil {
		selfdiagnostic.RegisterSelfDiagnosticHandler(ctx.DB)
		navicontacts.RegisterContactsHandler("navi-contacts", ctx.DB)
		documentknowledge.RegisterDocumentKnowledgeHandlers("document-knowledge", documentknowledge.DocumentKnowledgeHandlerConfig{
			DB:             ctx.DB,
			LLM:            ctx.LLM,
			Model:          ctx.Model,
			WorkspaceDir:   ctx.WorkspaceDir,
			ResolveOwnerID: ctx.ResolveOwnerID,
		})
	}
	naviprogrammer.RegisterProgrammerHandlers(naviprogrammer.RunValidationSkillID, naviprogrammer.ProgrammerHandlerConfig{
		DB:           ctx.DB,
		WorkspaceDir: ctx.WorkspaceDir,
	})
	navicoder.RegisterCoderRepoHandlers(navicoder.CoderRepoSkillID)
	if ctx.SkillBuilder != nil {
		skillcreator.RegisterSkillCreatorHandler("skill-creator", ctx.SkillBuilder, ctx.DB)
	}
}
