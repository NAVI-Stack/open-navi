package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var telegramAccountNamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// ResolvedTelegramAccount is a normalized Telegram account ready for connector wiring.
type ResolvedTelegramAccount struct {
	AccountName   string
	ConnectorName string
	BotToken      string
	OwnerChatID   int64
	AllowFrom     []int64
	PairingCode   string
	GatewayURL    string
	APIURL        string
	WebhookURL    string
	WebhookSecret string
	Groups        map[string]TelegramGroupConfig
}

// ResolveTelegramAccountToken resolves one account token from bot_token or token_file.
func ResolveTelegramAccountToken(cfg TelegramAccountConfig) (string, error) {
	return resolveTelegramToken(cfg.BotToken, cfg.TokenFile)
}

// ResolveTelegramAccounts normalizes Telegram account config and validates duplicate names/tokens.
// When Accounts is empty, it falls back to legacy single-account fields.
func ResolveTelegramAccounts(cfg TelegramConfig) ([]ResolvedTelegramAccount, error) {
	rawAccounts := cfg.Accounts
	useLegacy := len(rawAccounts) == 0
	if useLegacy {
		token, err := resolveTelegramToken(cfg.BotToken, cfg.TokenFile)
		if err != nil {
			return nil, err
		}
		if token == "" {
			return nil, nil
		}
		rawAccounts = []TelegramAccountConfig{
			{
				Name:          "default",
				BotToken:      token,
				OwnerChatID:   cfg.OwnerChatID,
				AllowFrom:     cfg.AllowFrom,
				PairingCode:   cfg.PairingCode,
				GatewayURL:    cfg.GatewayURL,
				APIURL:        cfg.APIURL,
				WebhookURL:    cfg.WebhookURL,
				WebhookSecret: cfg.WebhookSecret,
				Groups:        cfg.Groups,
			},
		}
	}

	seenNames := make(map[string]struct{}, len(rawAccounts))
	seenTokens := make(map[string]string, len(rawAccounts))
	out := make([]ResolvedTelegramAccount, 0, len(rawAccounts))

	for i, acct := range rawAccounts {
		accountName, err := normalizeTelegramAccountName(acct.Name, i, len(rawAccounts), useLegacy)
		if err != nil {
			return nil, err
		}
		connectorName := TelegramConnectorName(accountName)
		if _, exists := seenNames[connectorName]; exists {
			return nil, fmt.Errorf("duplicate telegram account name resolves to %q", connectorName)
		}

		token, err := resolveTelegramToken(acct.BotToken, acct.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("telegram account %q: %w", connectorName, err)
		}
		if token == "" {
			return nil, fmt.Errorf("telegram account %q: bot_token or token_file is required", connectorName)
		}
		if prev, exists := seenTokens[token]; exists {
			return nil, fmt.Errorf("duplicate telegram bot token for %q and %q", prev, connectorName)
		}
		seenTokens[token] = connectorName

		gatewayURL := strings.TrimSpace(acct.GatewayURL)
		if gatewayURL == "" {
			gatewayURL = strings.TrimSpace(cfg.GatewayURL)
		}
		if gatewayURL == "" {
			gatewayURL = "http://localhost:6284"
		}
		apiURL := strings.TrimSpace(acct.APIURL)
		if apiURL == "" {
			apiURL = strings.TrimSpace(cfg.APIURL)
		}
		if apiURL == "" {
			apiURL = "https://api.telegram.org"
		}

		out = append(out, ResolvedTelegramAccount{
			AccountName:   accountName,
			ConnectorName: connectorName,
			BotToken:      token,
			OwnerChatID:   acct.OwnerChatID,
			AllowFrom:     dedupeTelegramChatIDs(acct.AllowFrom),
			PairingCode:   strings.TrimSpace(acct.PairingCode),
			GatewayURL:    gatewayURL,
			APIURL:        apiURL,
			WebhookURL:    strings.TrimSpace(acct.WebhookURL),
			WebhookSecret: strings.TrimSpace(acct.WebhookSecret),
			Groups:        resolveTelegramGroups(acct.Groups, cfg.Groups),
		})
		seenNames[connectorName] = struct{}{}
	}

	return out, nil
}

// TelegramConnectorName returns the registry/connector name for a Telegram account.
func TelegramConnectorName(accountName string) string {
	n := strings.ToLower(strings.TrimSpace(accountName))
	if n == "" || n == "default" || n == "telegram" {
		return "telegram"
	}
	return "telegram-" + n
}

// ParseTelegramAccountsJSON parses persisted telegram_accounts JSON.
func ParseTelegramAccountsJSON(raw string) ([]TelegramAccountConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out []TelegramAccountConfig
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse telegram accounts: %w", err)
	}
	return out, nil
}

// EncodeTelegramAccountsJSON encodes Telegram accounts for persisted settings.
func EncodeTelegramAccountsJSON(accounts []TelegramAccountConfig) (string, error) {
	if len(accounts) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(accounts)
	if err != nil {
		return "", fmt.Errorf("encode telegram accounts: %w", err)
	}
	return string(b), nil
}

func normalizeTelegramAccountName(name string, idx, total int, legacy bool) (string, error) {
	if legacy {
		return "default", nil
	}

	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		if total == 1 {
			return "default", nil
		}
		return "", fmt.Errorf("telegram account[%d] missing name; use default|telegram or a unique slug", idx)
	}
	if n == "default" || n == "telegram" {
		return "default", nil
	}
	if !telegramAccountNamePattern.MatchString(n) {
		return "", fmt.Errorf("telegram account name %q is invalid: use [a-z0-9_-]+", name)
	}
	return n, nil
}

func resolveTelegramToken(botToken, tokenFile string) (string, error) {
	if botToken != "" {
		return strings.TrimSpace(botToken), nil
	}
	if tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("telegram token_file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", nil
}

// resolveTelegramGroups returns the account-level group config when set, otherwise
// the channel-level fallback. Account config fully overrides; there is no per-key merge.
func resolveTelegramGroups(accountGroups, fallback map[string]TelegramGroupConfig) map[string]TelegramGroupConfig {
	if len(accountGroups) > 0 {
		return accountGroups
	}
	return fallback
}

func dedupeTelegramChatIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func parseTelegramChatIDList(raw string) []int64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		idStr := strings.TrimSpace(part)
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
