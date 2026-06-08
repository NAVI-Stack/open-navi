package telegram

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/open-navi/navi/connectors"
	intconnectors "github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/intake"
	navicore "github.com/open-navi/navi/internal/navi"
	"github.com/open-navi/navi/internal/schema"
)

// GroupConfig holds per-group behavior. Key "*" sets defaults for all groups; a
// specific group chat ID (as a string) overrides the wildcard.
type GroupConfig struct {
	Enabled        *bool
	RequireMention *bool // default true in groups
}

type Config struct {
	Name            string
	TelegramToken   string
	GatewayURL      string
	GatewaySecret   string
	OwnerChatID     int64
	AllowFrom       []int64
	PairingCode     string
	APIURL          string
	WebhookURL      string // when set, use webhook instead of long-polling
	WebhookSecret   string // optional; sent as X-Telegram-Bot-Api-Secret-Token, validated in HandleWebhook
	Groups          map[string]GroupConfig
	SaveErrorRecord func(ctx context.Context, component, naviChatID, runID, errorType, message, contextJSON string) error
	// IntakeEmit, when non-nil, is called with a serialised IntakeRecord for
	// each inbound message. Failure is logged but never blocks message delivery.
	IntakeEmit func(ctx context.Context, data []byte) error
	apiURL     string // overridden in tests
}

type Bot struct {
	cfg                    Config
	name                   string
	token                  string
	botUsername            string
	baseURL                string
	client                 *http.Client
	done                   chan struct{}
	closeDoneOnce          sync.Once
	running                bool
	useWebhook             bool
	activeDirectiveID      string
	activeChatID           string
	mu                     sync.Mutex
	notifiedHITL           map[string]bool
	allowedChats           map[int64]bool
	chatDirective          map[int64]string // Telegram chat_id -> active directive_id
	telegramChatToNaviChat map[int64]string // Telegram chat_id -> active NAVI chat_id
	naviChatToTelegramChat map[string]int64 // NAVI chat_id -> Telegram chat_id for reply routing
	telegramChatToEndpoint map[telegramEndpointKey]string
	chatStream             map[string]streamMessage
	chatRepair             map[string]int64
	chatWatchdog           map[string]bool
	chatTypingStop         map[string]func()
	directiveChat          map[string]int64 // directive_id -> Telegram chat_id for reply routing
	freshChatIDs           map[string]bool  // sessions created by this bot instance (not recovered); used to control ws replay
	newHITLTickSource      func() (<-chan time.Time, func())
	chatBackoff            func(attempt int) time.Duration
	completionRepairGrace  time.Duration
	terminalWatchdogPoll   time.Duration
	sleep                  func(time.Duration)
	emitIntake             func(ctx context.Context, record []byte) error // nil = disabled
}

type telegramEndpointKey struct {
	ChatID   int64
	ThreadID int64
}

type streamMessage struct {
	ChatID    int64
	MessageID int64
	Content   string
}

func isInternalRuntimeChatID(naviChatID string) bool {
	return schema.IsInternalRuntimeSessionID(naviChatID)
}

var telegramBreakTagPattern = regexp.MustCompile(`(?i)<br\s*/?>`)

func NewBot(cfg Config) *Bot {
	if cfg.apiURL == "" {
		cfg.apiURL = strings.TrimSpace(cfg.APIURL)
	}
	if cfg.apiURL == "" {
		cfg.apiURL = "https://api.telegram.org"
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = "telegram"
	}
	allowedChats := make(map[int64]bool, len(cfg.AllowFrom)+1)
	if cfg.OwnerChatID != 0 {
		allowedChats[cfg.OwnerChatID] = true
	}
	for _, id := range cfg.AllowFrom {
		if id != 0 {
			allowedChats[id] = true
		}
	}
	return &Bot{
		cfg:                    cfg,
		name:                   name,
		baseURL:                fmt.Sprintf("%s/bot%s", cfg.apiURL, cfg.TelegramToken),
		client:                 &http.Client{Timeout: 60 * time.Second},
		done:                   make(chan struct{}),
		useWebhook:             cfg.WebhookURL != "",
		notifiedHITL:           make(map[string]bool),
		allowedChats:           allowedChats,
		chatDirective:          make(map[int64]string),
		telegramChatToNaviChat: make(map[int64]string),
		naviChatToTelegramChat: make(map[string]int64),
		telegramChatToEndpoint: make(map[telegramEndpointKey]string),
		chatStream:             make(map[string]streamMessage),
		chatRepair:             make(map[string]int64),
		chatWatchdog:           make(map[string]bool),
		chatTypingStop:         make(map[string]func()),
		directiveChat:          make(map[string]int64),
		freshChatIDs:           make(map[string]bool),
		newHITLTickSource: func() (<-chan time.Time, func()) {
			ticker := time.NewTicker(60 * time.Second)
			return ticker.C, ticker.Stop
		},
		chatBackoff:           defaultChatBackoff,
		completionRepairGrace: defaultCompletionRepairGrace,
		terminalWatchdogPoll:  defaultTerminalWatchdogPoll,
		sleep:                 time.Sleep,
		emitIntake:            cfg.IntakeEmit,
	}
}

// SetIntakeEmitter wires an intake emission callback into the bot. fn receives
// a JSON-serialised IntakeRecord and publishes it to navi.refinery.queue.
// Call before Start(). Passing nil disables emission (default).
func (b *Bot) SetIntakeEmitter(fn func(ctx context.Context, record []byte) error) {
	b.emitIntake = fn
}

// buildIntakeRecord constructs and serialises an IntakeRecord for an inbound
// Telegram message. Returns nil if msg is nil or carries no usable content.
func (b *Bot) buildIntakeRecord(msg *tgMessage) []byte {
	if msg == nil {
		return nil
	}
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	if text == "" && msg.Document == nil && len(msg.Photo) == 0 &&
		msg.Video == nil && msg.Audio == nil && msg.Voice == nil {
		return nil
	}

	trust := intake.TrustExternalUntrusted
	// In Telegram, a private chat's chat ID equals the user ID. Compare the
	// sender's user ID against OwnerChatID to identify owner messages.
	if msg.From != nil && b.cfg.OwnerChatID != 0 && msg.From.ID == b.cfg.OwnerChatID {
		trust = intake.TrustOwner
	}

	author := ""
	if msg.From != nil {
		if msg.From.Username != "" {
			author = "@" + msg.From.Username
		} else {
			author = strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
		}
	}

	connectorID := "telegram:" + b.name
	sourceID := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageID)

	r := intake.IntakeRecord{
		ConnectorID:  connectorID,
		SourceKind:   "message",
		SourceID:     sourceID,
		Cursor:       sourceID,
		FetchedAt:    time.Now().UTC(),
		Trust:        trust,
		PrivacyClass: intake.PrivacyPersonal,
		Author:       author,
		Raw:          []byte(text),
		RawMIME:      "text/plain",
		Provenance: intake.Provenance{
			ConnectorID: connectorID,
			AccountID:   b.name,
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		slog.Warn("telegram: intake: marshal failed", "err", err)
		return nil
	}
	return data
}

func defaultChatBackoff(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 2 * time.Second
	case 1:
		return 4 * time.Second
	default:
		return 8 * time.Second
	}
}

func (b *Bot) multiChatModeLocked() bool {
	if strings.TrimSpace(b.cfg.PairingCode) != "" {
		return true
	}
	return len(b.allowedChats) > 1
}

func (b *Bot) sessionFocusForChatLocked(chatID int64) string {
	if chatID == 0 {
		return ""
	}
	naviChatID := strings.TrimSpace(b.telegramChatToNaviChat[chatID])
	if naviChatID == "" && !b.multiChatModeLocked() {
		naviChatID = strings.TrimSpace(b.activeChatID)
	}
	if isInternalRuntimeChatID(naviChatID) {
		return ""
	}
	return naviChatID
}

func (b *Bot) directiveFocusForChatLocked(chatID int64) string {
	if chatID == 0 {
		return ""
	}
	directiveID := strings.TrimSpace(b.chatDirective[chatID])
	if directiveID == "" && !b.multiChatModeLocked() {
		directiveID = strings.TrimSpace(b.activeDirectiveID)
	}
	return directiveID
}

func (b *Bot) bindSessionToChat(chatID int64, naviChatID string) {
	naviChatID = strings.TrimSpace(naviChatID)
	if chatID == 0 || naviChatID == "" || isInternalRuntimeChatID(naviChatID) {
		return
	}
	b.mu.Lock()
	b.telegramChatToNaviChat[chatID] = naviChatID
	b.naviChatToTelegramChat[naviChatID] = chatID
	b.activeChatID = naviChatID
	b.mu.Unlock()
}

func (b *Bot) bindEndpointToChat(chatID, threadID int64, endpointID string) {
	endpointID = strings.TrimSpace(endpointID)
	if chatID == 0 || endpointID == "" {
		return
	}
	b.mu.Lock()
	b.telegramChatToEndpoint[telegramEndpointKey{ChatID: chatID, ThreadID: threadID}] = endpointID
	b.mu.Unlock()
}

func (b *Bot) endpointIDForChat(chatID, threadID int64) string {
	if chatID == 0 {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.telegramChatToEndpoint[telegramEndpointKey{ChatID: chatID, ThreadID: threadID}])
}

func (b *Bot) bindDirectiveToChat(chatID int64, directiveID string) {
	directiveID = strings.TrimSpace(directiveID)
	if chatID == 0 || directiveID == "" {
		return
	}
	b.mu.Lock()
	b.chatDirective[chatID] = directiveID
	b.directiveChat[directiveID] = chatID
	b.activeDirectiveID = directiveID
	b.mu.Unlock()
}

func (b *Bot) clearDirectiveFocus(chatID int64) {
	if chatID == 0 {
		return
	}
	b.mu.Lock()
	delete(b.chatDirective, chatID)
	if !b.multiChatModeLocked() {
		b.activeDirectiveID = ""
	}
	b.mu.Unlock()
}

func (b *Bot) chatIDForSession(naviChatID string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.naviChatToTelegramChat[naviChatID]
}

func (b *Bot) sessionIDForChat(chatID int64) string {
	if chatID == 0 {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionFocusForChatLocked(chatID)
}

func (b *Bot) recordConnectorError(ctx context.Context, naviChatID, errorType, message string, payload map[string]any) {
	if b.cfg.SaveErrorRecord == nil {
		return
	}
	if strings.TrimSpace(message) == "" {
		return
	}
	if payload == nil {
		payload = make(map[string]any, 1)
	}
	payload["connector"] = b.Name()
	contextJSONBytes, err := json.Marshal(payload)
	if err != nil {
		contextJSONBytes = []byte(`{"connector":"` + b.Name() + `"}`)
	}
	_ = b.cfg.SaveErrorRecord(ctx, "connector", strings.TrimSpace(naviChatID), "", errorType, message, string(contextJSONBytes))
}

func (b *Bot) liveSessionSnapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := make(map[string]struct{}, len(b.telegramChatToNaviChat)+len(b.chatStream)+len(b.chatTypingStop))
	add := func(naviChatID string) {
		naviChatID = strings.TrimSpace(naviChatID)
		if naviChatID == "" || isInternalRuntimeChatID(naviChatID) {
			return
		}
		seen[naviChatID] = struct{}{}
	}
	for _, naviChatID := range b.telegramChatToNaviChat {
		add(naviChatID)
	}
	for naviChatID := range b.chatStream {
		add(naviChatID)
	}
	for naviChatID := range b.chatTypingStop {
		add(naviChatID)
	}
	out := make([]string, 0, len(seen))
	for naviChatID := range seen {
		if b.naviChatToTelegramChat[naviChatID] == 0 {
			continue
		}
		out = append(out, naviChatID)
	}
	return out
}

func (b *Bot) chatStreamState(naviChatID string) (token string, chatID int64, isFresh bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.token, b.naviChatToTelegramChat[naviChatID], b.freshChatIDs[naviChatID]
}

func (b *Bot) clearFreshSession(naviChatID string) {
	if strings.TrimSpace(naviChatID) == "" {
		return
	}
	b.mu.Lock()
	delete(b.freshChatIDs, naviChatID)
	b.mu.Unlock()
}

func (b *Bot) waitForLiveRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-b.done:
		return false
	case <-timer.C:
		return true
	}
}

func (b *Bot) Name() string { return b.name }

// ManagesInboundOrchestration tells the gateway to skip the generic connector
// manager indicator flow for Telegram chat messages because this bot owns
// its typing/placeholder/streaming lifecycle via /ws/live.
func (b *Bot) ManagesInboundOrchestration() bool { return true }

// MaxMessageLength implements connectors.MessageLengthProvider. Telegram's limit is 4096.
func (b *Bot) MaxMessageLength() int { return 4096 }

// Category implements connectors.Categorizable. Telegram is a Communication connector.
func (b *Bot) Category() string { return connectors.CategoryCommunication }

// WebhookPath implements connectors.WebhookHandler. Used when WebhookURL is set.
func (b *Bot) WebhookPath() string { return "/webhooks/" + b.Name() }

// HandleWebhook implements connectors.WebhookHandler. Parses Telegram update and handles it.
func (b *Bot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.cfg.WebhookSecret != "" {
		secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if secret != b.cfg.WebhookSecret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var u tgUpdate
	if err := json.Unmarshal(body, &u); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	go b.handleUpdate(context.Background(), u)
}

// HealthCheck implements connectors.HealthChecker by calling Telegram getMe.
func (b *Bot) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/getMe", nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram getMe: %d %s", resp.StatusCode, string(body))
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("telegram getMe: ok=false")
	}
	return nil
}

// fetchBotIdentity calls getMe and caches the bot's username, used for mention
// detection in group chats. Best-effort: failures leave botUsername empty.
func (b *Bot) fetchBotIdentity(ctx context.Context) {
	var out struct {
		Username string `json:"username"`
	}
	if err := b.tgCall(ctx, "getMe", map[string]any{}, &out); err != nil {
		slog.Warn("telegram: getMe for bot identity failed", "error", err)
		return
	}
	b.mu.Lock()
	b.botUsername = strings.TrimSpace(out.Username)
	b.mu.Unlock()
}

type tgBotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// telegramCommandMenu returns the slash-command list registered with Telegram so
// clients show autocomplete. /pair is included only when pairing is enabled.
func (b *Bot) telegramCommandMenu() []tgBotCommand {
	cmds := []tgBotCommand{
		{Command: "navi", Description: "Start or refocus a NAVI chat session"},
		{Command: "new", Description: "Create a directive: /new <title>"},
		{Command: "use", Description: "Set the active directive: /use <directive_id>"},
		{Command: "list", Description: "List recent directives"},
		{Command: "implement", Description: "Set a directive to ACT mode: /implement <id>"},
		{Command: "status", Description: "Show agent and system status"},
		{Command: "model", Description: "Manage the LLM model: /model list|current|set"},
		{Command: "hitl", Description: "List pending approvals"},
	}
	if strings.TrimSpace(b.cfg.PairingCode) != "" {
		cmds = append(cmds, tgBotCommand{Command: "pair", Description: "Authorize this chat: /pair <code>"})
	}
	return cmds
}

// registerCommandMenu publishes the slash-command list via setMyCommands. Best-effort:
// failures are logged and recorded but do not block startup.
func (b *Bot) registerCommandMenu(ctx context.Context) {
	if err := b.tgCall(ctx, "setMyCommands", map[string]any{"commands": b.telegramCommandMenu()}, nil); err != nil {
		slog.Warn("telegram: setMyCommands failed", "error", err)
		b.recordConnectorError(ctx, "", "command_menu_registration_failed", err.Error(), map[string]any{"phase": "startup"})
	}
}

func (b *Bot) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

func (b *Bot) Send(ctx context.Context, msg connectors.OutboundMessage) error {
	if !b.IsRunning() {
		return fmt.Errorf("telegram: %w", connectors.ErrNotRunning)
	}
	chatID := msg.ChatID
	if chatID == "" {
		chatID = strconv.FormatInt(b.cfg.OwnerChatID, 10)
	}
	if _, err := b.sendMessageToChat(ctx, chatID, msg.Content, nil, msg.ReplyToMessageID, msg.MessageThreadID, msg.ParseMode); err != nil {
		return fmt.Errorf("telegram: %w: %v", connectors.ErrSendFailed, err)
	}
	return nil
}

const telegramThinkingPlaceholder = "Thinking... \U0001F4AD"

const defaultCompletionRepairGrace = 350 * time.Millisecond
const defaultTerminalWatchdogPoll = 2 * time.Second

// defaultTerminalWatchdogTimeout is the maximum time the watchdog will wait for
// a new run to complete before giving up and clearing the thinking indicator.
// Set to 7 minutes — slightly over the coordinator run timeout of ~5m30s.
const defaultTerminalWatchdogTimeout = 7 * time.Minute

type sessionRuntimeSummary struct {
	ChatID        string                       `json:"chat_id"`
	Run           *sessionRuntimeSummaryRun    `json:"run,omitempty"`
	TerminalEvent *sessionRuntimeTerminalEvent `json:"terminal_event,omitempty"`
}

type sessionRuntimeSummaryRun struct {
	RunID     string    `json:"run_id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

type sessionRuntimeTerminalEvent struct {
	Type    string         `json:"type"`
	Seq     int64          `json:"seq"`
	Payload map[string]any `json:"payload"`
}

type modelCommand struct {
	Handled  bool
	Action   string
	Provider string
	Model    string
	Usage    string
}

type telegramLLMCatalog struct {
	Providers []struct {
		Key         string `json:"key"`
		DisplayName string `json:"display_name"`
		Models      []struct {
			Name string `json:"name"`
		} `json:"models"`
	} `json:"providers"`
}

// StartTyping implements connectors.TypingCapable using Telegram's sendChatAction.
func (b *Bot) StartTyping(ctx context.Context, chatID string) (func(), error) {
	parsedChatID, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, err
	}
	if err := b.sendChatAction(ctx, parsedChatID, "typing"); err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(4 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := b.sendChatAction(context.Background(), parsedChatID, "typing"); err != nil {
					log.Printf("telegram: sendChatAction failed: %v", err)
				}
			}
		}
	}()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(cancel)
	}, nil
}

// SendPlaceholder implements connectors.PlaceholderCapable by sending a
// temporary placeholder message that can later be edited in place.
func (b *Bot) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	messageID, err := b.sendMessageToChat(ctx, chatID, telegramThinkingPlaceholder, nil, 0, 0, "")
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(messageID, 10), nil
}

// EditMessage implements connectors.MessageEditor for Telegram.
func (b *Bot) EditMessage(ctx context.Context, chatID, messageID, content string) error {
	parsedChatID, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	parsedMessageID, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return err
	}
	return b.editMessageForChat(ctx, parsedChatID, parsedMessageID, content)
}

// SendMedia implements connectors.MediaSender.
// Images use sendPhoto, all other payloads use sendDocument.
func (b *Bot) SendMedia(ctx context.Context, msg connectors.OutboundMediaMessage) error {
	if !b.IsRunning() {
		return fmt.Errorf("telegram: %w", connectors.ErrNotRunning)
	}
	if len(msg.Parts) == 0 {
		return fmt.Errorf("telegram: %w: media message has no parts", connectors.ErrSendFailed)
	}
	chatID := msg.ChatID
	if chatID == "" {
		chatID = strconv.FormatInt(b.cfg.OwnerChatID, 10)
	}
	for _, part := range msg.Parts {
		if err := b.sendMediaPartToChat(ctx, chatID, part); err != nil {
			return fmt.Errorf("telegram: %w: %v", connectors.ErrSendFailed, err)
		}
	}
	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	b.mu.Lock()
	b.running = true
	b.mu.Unlock()
	if err := b.refreshTokenWithBackoff(ctx); err != nil {
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
		return fmt.Errorf("telegram: initial auth failed: %w", err)
	}
	b.fetchBotIdentity(ctx)
	b.registerCommandMenu(ctx)
	if err := b.waitForGatewayReady(ctx, 10*time.Second); err != nil {
		log.Printf("telegram: gateway readiness check failed: %v", err)
		b.recordConnectorError(ctx, "", "gateway_readiness_failed", err.Error(), map[string]any{"phase": "startup"})
	}

	_ = b.registerConnector(ctx)

	go b.hitlPoller(ctx)
	go b.wsLivePoller(ctx)

	// Attempt to recover focus from last session/directive
	if err := b.recoverFocus(ctx); err != nil {
		log.Printf("telegram: focus recovery failed: %v", err)
		b.recordConnectorError(ctx, "", "focus_recovery_failed", err.Error(), map[string]any{"phase": "startup"})
	}

	go func() {
		// refresh token every 20h (token ttl is 24h)
		tkTicker := time.NewTicker(20 * time.Hour)
		defer tkTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.done:
				return
			case <-tkTicker.C:
				_ = b.refreshTokenWithBackoff(ctx)
			}
		}
	}()

	if b.useWebhook {
		if err := b.setWebhook(ctx); err != nil {
			log.Printf("telegram: setWebhook failed: %v", err)
			b.recordConnectorError(ctx, "", "webhook_registration_failed", err.Error(), map[string]any{"phase": "startup"})
			return fmt.Errorf("telegram: setWebhook: %w", err)
		}
		log.Printf("telegram: webhook registered at %s", b.cfg.WebhookURL)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.done:
			return nil
		}
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.done:
			return nil
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			b.handleUpdate(ctx, u)
		}
	}
}

func (b *Bot) Stop(ctx context.Context) error {
	b.mu.Lock()
	b.running = false
	b.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if b.useWebhook {
		_ = b.deleteWebhook(ctx)
	}
	_ = b.deregisterConnector(ctx)
	b.closeDoneOnce.Do(func() { close(b.done) })
	return nil
}

// Telegram registry proxy calls
func (b *Bot) registerConnector(ctx context.Context) error {
	reqBody, _ := json.Marshal(map[string]string{"name": b.Name()})
	req, _ := http.NewRequestWithContext(ctx, "POST", b.cfg.GatewayURL+"/api/connectors", bytes.NewReader(reqBody))
	req.Header.Set("X-API-Key", b.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	return nil
}

func (b *Bot) deregisterConnector(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "DELETE", b.cfg.GatewayURL+"/api/connectors/"+b.Name(), nil)
	req.Header.Set("X-API-Key", b.token)
	resp, err := b.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	return nil
}

func (b *Bot) setWebhook(ctx context.Context) error {
	payload := map[string]any{
		"url":             b.cfg.WebhookURL,
		"allowed_updates": telegramAllowedUpdates,
	}
	if b.cfg.WebhookSecret != "" {
		payload["secret_token"] = b.cfg.WebhookSecret
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/setWebhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setWebhook: %d %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (b *Bot) deleteWebhook(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/deleteWebhook", nil)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// refreshTokenWithBackoff is a no-op in the API-key model — kept for call-site
// compatibility. The GatewaySecret is used directly as an API key in every request.
func (b *Bot) refreshTokenWithBackoff(_ context.Context) error {
	if b.cfg.GatewaySecret == "" {
		log.Printf("telegram: warning — GatewaySecret is empty; gateway calls will be unauthenticated")
	}
	b.token = b.cfg.GatewaySecret
	return nil
}

// gatewayDo simplifies calling the gateway API.
func (b *Bot) gatewayDo(ctx context.Context, method, path string, body any) ([]byte, error) {
	bBytes, status, err := b.doSingle(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("gateway %s %s: status %d %s", method, path, status, string(bBytes))
	}
	return bBytes, nil
}

type queuedSessionMessageResponse struct {
	Status              string `json:"status"`
	InboxItemID         string `json:"inbox_item_id"`
	QueueAction         string `json:"queue_action"`
	InboxStatus         string `json:"inbox_status"`
	ClassifiedReason    string `json:"classified_reason"`
	BlockedOnProposalID string `json:"blocked_on_proposal_id"`
	PauseReason         string `json:"pause_reason"`
}

func (b *Bot) queueSessionMessage(ctx context.Context, naviChatID, content, sourceRef, idempotencyKey, originEndpointID string) (queuedSessionMessageResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		res, err := b.queueSessionMessageOnce(ctx, naviChatID, content, sourceRef, idempotencyKey, originEndpointID)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if attempt == 2 || !isRetryableQueueSessionMessageError(err) {
			break
		}
		delay := defaultChatBackoff(attempt)
		if b.chatBackoff != nil {
			delay = b.chatBackoff(attempt)
		}
		if b.sleep != nil {
			b.sleep(delay)
		} else {
			time.Sleep(delay)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("telegram: queue chat message failed")
	}
	return queuedSessionMessageResponse{}, lastErr
}

func (b *Bot) queueSessionMessageOnce(ctx context.Context, naviChatID, content, sourceRef, idempotencyKey, originEndpointID string) (queuedSessionMessageResponse, error) {
	payload := map[string]any{
		"content":            content,
		"source_channel":     b.Name(),
		"source_message_ref": sourceRef,
		"idempotency_key":    idempotencyKey,
	}
	if originEndpointID = strings.TrimSpace(originEndpointID); originEndpointID != "" {
		payload["origin_endpoint_id"] = originEndpointID
	}
	data, err := b.gatewayDo(ctx, "POST", "/api/navi/chats/"+naviChatID+"/message", payload)
	if err != nil {
		return queuedSessionMessageResponse{}, err
	}
	var res queuedSessionMessageResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return queuedSessionMessageResponse{}, fmt.Errorf("telegram: decode queued chat message response: %w", err)
	}
	return res, nil
}

func isRetryableQueueSessionMessageError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	lowered := strings.ToLower(err.Error())
	if strings.Contains(lowered, "timeout") || strings.Contains(lowered, "tempor") || strings.Contains(lowered, "connection reset") {
		return true
	}
	for _, status := range []string{"status 408", "status 429", "status 500", "status 502", "status 503", "status 504"} {
		if strings.Contains(lowered, status) {
			return true
		}
	}
	return false
}

func queuedMessageStartsThinking(res queuedSessionMessageResponse) bool {
	switch strings.TrimSpace(res.InboxStatus) {
	case "", "pending", "merged":
		return true
	case "deferred", "superseded":
		return false
	default:
		return strings.TrimSpace(res.QueueAction) != "defer" && strings.TrimSpace(res.QueueAction) != "supersede"
	}
}

func queuedMessageDeferredNotice(res queuedSessionMessageResponse) (string, any, bool) {
	if strings.TrimSpace(res.InboxStatus) != "deferred" && strings.TrimSpace(res.QueueAction) != "defer" {
		return "", nil, false
	}
	lines := []string{"This session is waiting for earlier input before it can continue."}
	if proposalID := strings.TrimSpace(res.BlockedOnProposalID); proposalID != "" {
		lines[0] = "Approval needed before this chat can continue."
		lines = append(lines, "Proposal: "+proposalID)
		reason := strings.TrimSpace(res.PauseReason)
		if reason == "" {
			reason = strings.TrimSpace(res.ClassifiedReason)
		}
		if reason != "" {
			lines = append(lines, "Reason: "+reason)
		}
		return strings.Join(lines, "\n"), proposalReplyMarkup(proposalID), true
	}
	if reason := strings.TrimSpace(res.ClassifiedReason); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	return strings.Join(lines, "\n"), nil, true
}

func (b *Bot) waitForGatewayReady(ctx context.Context, timeoutOverride ...time.Duration) error {
	if strings.TrimSpace(b.cfg.GatewayURL) == "" {
		return fmt.Errorf("gateway url is empty")
	}
	timeout := 30 * time.Second
	if len(timeoutOverride) > 0 && timeoutOverride[0] > 0 {
		timeout = timeoutOverride[0]
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	for attempt := 1; ; attempt++ {
		reqCtx, reqCancel := context.WithTimeout(waitCtx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, b.cfg.GatewayURL+"/health", nil)
		if err != nil {
			reqCancel()
			return err
		}
		if b.token != "" {
			req.Header.Set("X-API-Key", b.token)
		}

		resp, err := b.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				reqCancel()
				return nil
			}
			lastErr = fmt.Errorf("health status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		reqCancel()

		if waitCtx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return waitCtx.Err()
		}

		backoff := time.Duration(attempt) * 500 * time.Millisecond
		if backoff > 3*time.Second {
			backoff = 3 * time.Second
		}
		if b.sleep != nil {
			b.sleep(backoff)
		} else {
			time.Sleep(backoff)
		}
	}
}

func (b *Bot) doSingle(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		j, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(j)
	}
	req, _ := http.NewRequestWithContext(ctx, method, b.cfg.GatewayURL+path, bodyReader)
	req.Header.Set("X-API-Key", b.token) // b.token == GatewaySecret (API key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	bBytes, _ := io.ReadAll(resp.Body)
	return bBytes, resp.StatusCode, nil
}

func (b *Bot) recoverFocus(ctx context.Context) error {
	// 1. Try to find active NAVI session
	data, err := b.gatewayDo(ctx, "GET", "/api/navi/chats", nil)
	if err == nil {
		var sessions []struct {
			ChatID string `json:"chat_id"`
		}
		if json.Unmarshal(data, &sessions) == nil && len(sessions) > 0 {
			for _, session := range sessions {
				id := strings.TrimSpace(session.ChatID)
				if id == "" || isInternalRuntimeChatID(id) {
					continue
				}
				b.mu.Lock()
				b.activeChatID = id
				// Only auto-bind recovered chats when the bot is effectively single-chat.
				if b.cfg.OwnerChatID != 0 && !b.multiChatModeLocked() {
					b.telegramChatToNaviChat[b.cfg.OwnerChatID] = id
					if _, exists := b.naviChatToTelegramChat[id]; !exists {
						b.naviChatToTelegramChat[id] = b.cfg.OwnerChatID
					}
				}
				b.mu.Unlock()
				log.Printf("telegram: recovered NAVI focus to session %s", id)
				return nil
			}
		}
	}

	// 2. Try to find active Orchestrator directive
	data, err = b.gatewayDo(ctx, "GET", "/api/directives", nil)
	if err == nil {
		var directives []map[string]any
		if json.Unmarshal(data, &directives) == nil && len(directives) > 0 {
			latest := directives[0]
			id, _ := latest["directive_id"].(string)
			if id != "" {
				b.mu.Lock()
				b.activeDirectiveID = id
				if b.cfg.OwnerChatID != 0 && !b.multiChatModeLocked() {
					b.chatDirective[b.cfg.OwnerChatID] = id
					b.directiveChat[id] = b.cfg.OwnerChatID
				}
				b.mu.Unlock()
				log.Printf("telegram: recovered Orchestrator focus to directive %s", id)
				return nil
			}
		}
	}

	return nil
}

func (b *Bot) recoverSessionForChat(ctx context.Context, chatID int64) (string, error) {
	if chatID == 0 {
		return "", nil
	}
	b.mu.Lock()
	multiChat := b.multiChatModeLocked()
	b.mu.Unlock()
	if multiChat {
		return "", nil
	}

	data, err := b.gatewayDo(ctx, "GET", "/api/navi/chats", nil)
	if err != nil {
		return "", err
	}
	var sessions []struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(data, &sessions); err != nil {
		return "", err
	}
	for _, session := range sessions {
		id := strings.TrimSpace(session.ChatID)
		if id == "" || isInternalRuntimeChatID(id) {
			continue
		}
		b.bindSessionToChat(chatID, id)
		log.Printf("telegram: recovered NAVI focus to session %s for chat %d", id, chatID)
		return id, nil
	}
	return "", nil
}

func (b *Bot) createChatWithRetry(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		data, err := b.gatewayDo(ctx, http.MethodPost, "/api/navi/chats", map[string]string{})
		if err == nil {
			var res map[string]string
			if json.Unmarshal(data, &res) == nil {
				if naviChatID := strings.TrimSpace(res["chat_id"]); naviChatID != "" {
					return naviChatID, nil
				}
			}
			lastErr = fmt.Errorf("telegram: create session response missing chat_id")
		} else {
			lastErr = err
		}

		if attempt == 2 {
			break
		}
		delay := defaultChatBackoff(attempt)
		if b.chatBackoff != nil {
			delay = b.chatBackoff(attempt)
		}
		if b.sleep != nil {
			b.sleep(delay)
		} else {
			time.Sleep(delay)
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("telegram: create session failed")
	}
	return "", lastErr
}

type telegramEndpointResolveResponse struct {
	ChatID     string                        `json:"chat_id"`
	EndpointID string                        `json:"endpoint_id"`
	Endpoint   navicore.ConversationEndpoint `json:"endpoint"`
}

func (b *Bot) resolveEndpointForMessage(ctx context.Context, msg *tgMessage) (telegramEndpointResolveResponse, error) {
	if msg == nil {
		return telegramEndpointResolveResponse{}, fmt.Errorf("telegram: message is required to resolve endpoint")
	}
	return b.resolveEndpointForChat(ctx, msg.Chat.ID, msg.MessageThreadID, telegramChatDisplayName(msg.Chat))
}

func (b *Bot) resolveEndpointForChat(ctx context.Context, chatID, threadID int64, displayName string) (telegramEndpointResolveResponse, error) {
	if chatID == 0 {
		return telegramEndpointResolveResponse{}, fmt.Errorf("telegram: chat_id is required to resolve endpoint")
	}
	req := map[string]string{
		"connector_kind":        "telegram",
		"connector_instance_id": b.Name(),
		"external_chat_id":      strconv.FormatInt(chatID, 10),
		"display_name":          strings.TrimSpace(displayName),
	}
	if threadID != 0 {
		req["external_thread_id"] = strconv.FormatInt(threadID, 10)
	}
	data, err := b.gatewayDo(ctx, http.MethodPost, "/api/connectors/endpoints/resolve", req)
	if err != nil {
		return telegramEndpointResolveResponse{}, err
	}
	var res telegramEndpointResolveResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return telegramEndpointResolveResponse{}, fmt.Errorf("telegram: decode endpoint resolve response: %w", err)
	}
	res.ChatID = strings.TrimSpace(res.ChatID)
	res.EndpointID = strings.TrimSpace(res.EndpointID)
	if res.EndpointID == "" {
		res.EndpointID = strings.TrimSpace(string(res.Endpoint.ID))
	}
	if res.ChatID == "" && strings.TrimSpace(string(res.Endpoint.ChatID)) != "" {
		res.ChatID = strings.TrimSpace(string(res.Endpoint.ChatID))
	}
	if res.ChatID == "" || res.EndpointID == "" {
		return telegramEndpointResolveResponse{}, fmt.Errorf("telegram: endpoint resolve response missing chat_id or endpoint_id")
	}
	b.bindSessionToChat(chatID, res.ChatID)
	b.bindEndpointToChat(chatID, threadID, res.EndpointID)
	return res, nil
}

func (b *Bot) ensureChat(ctx context.Context, chatID int64) (string, error) {
	resolved, err := b.resolveEndpointForChat(ctx, chatID, 0, "")
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.activeChatID = resolved.ChatID
	if chatID != 0 {
		delete(b.chatDirective, chatID)
	}
	b.activeDirectiveID = ""
	b.mu.Unlock()
	return resolved.ChatID, nil
}

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	EditedMessage *tgMessage       `json:"edited_message"`
	ChannelPost   *tgMessage       `json:"channel_post"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type tgMessage struct {
	MessageID       int64            `json:"message_id"`
	MessageThreadID int64            `json:"message_thread_id,omitempty"`
	Date            int64            `json:"date"`
	Chat            tgChat           `json:"chat"`
	From            *tgUser          `json:"from"`
	Text            string           `json:"text"`
	Caption         string           `json:"caption"`
	Photo           []tgPhotoSize    `json:"photo"`
	Video           *tgVideo         `json:"video"`
	Audio           *tgAudio         `json:"audio"`
	Voice           *tgVoice         `json:"voice"`
	Document        *tgDocument      `json:"document"`
	Sticker         *tgSticker       `json:"sticker"`
	ReplyToMessage  *tgMessage       `json:"reply_to_message"`
	Entities        []tgEntity       `json:"entities"`
	ForwardOrigin   *tgForwardOrigin `json:"forward_origin"`
}

type tgEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// tgForwardOrigin captures the Telegram Bot API MessageOrigin union (user/chat/channel/hidden_user).
type tgForwardOrigin struct {
	Type            string  `json:"type"`
	SenderUser      *tgUser `json:"sender_user"`
	SenderUserName  string  `json:"sender_user_name"`
	SenderChat      *tgChat `json:"sender_chat"`
	Chat            *tgChat `json:"chat"`
	AuthorSignature string  `json:"author_signature"`
}

type tgChat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type tgPhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size"`
}

type tgVideo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type tgAudio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type tgVoice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type tgDocument struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type tgSticker struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	IsAnimated   bool   `json:"is_animated"`
	IsVideo      bool   `json:"is_video"`
	Emoji        string `json:"emoji"`
	FileSize     int64  `json:"file_size"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

type InboundAttachment struct {
	Source           string // "telegram"
	TelegramFileID   string
	TelegramFilePath string
	MediaKind        string // photo, voice, audio, video, document, sticker
	MIMEType         string
	Filename         string
	SizeBytes        int64
	LocalPath        string // optional
	DataBase64       string // optional, only for small inline media
	Caption          string
}

func (b *Bot) parseMediaMetadata(msg *tgMessage) []InboundAttachment {
	if msg == nil {
		return nil
	}
	var attachments []InboundAttachment

	if len(msg.Photo) > 0 {
		// Take the largest photo
		p := msg.Photo[len(msg.Photo)-1]
		attachments = append(attachments, InboundAttachment{
			Source:         "telegram",
			TelegramFileID: p.FileID,
			MediaKind:      "photo",
			SizeBytes:      p.FileSize,
			Caption:        msg.Caption,
		})
	}

	if msg.Video != nil {
		attachments = append(attachments, InboundAttachment{
			Source:         "telegram",
			TelegramFileID: msg.Video.FileID,
			MediaKind:      "video",
			MIMEType:       msg.Video.MimeType,
			SizeBytes:      msg.Video.FileSize,
			Caption:        msg.Caption,
		})
	}

	if msg.Audio != nil {
		attachments = append(attachments, InboundAttachment{
			Source:         "telegram",
			TelegramFileID: msg.Audio.FileID,
			MediaKind:      "audio",
			MIMEType:       msg.Audio.MimeType,
			SizeBytes:      msg.Audio.FileSize,
			Caption:        msg.Caption,
		})
	}

	if msg.Voice != nil {
		attachments = append(attachments, InboundAttachment{
			Source:         "telegram",
			TelegramFileID: msg.Voice.FileID,
			MediaKind:      "voice",
			MIMEType:       msg.Voice.MimeType,
			SizeBytes:      msg.Voice.FileSize,
			Caption:        msg.Caption,
		})
	}

	if msg.Document != nil {
		attachments = append(attachments, InboundAttachment{
			Source:         "telegram",
			TelegramFileID: msg.Document.FileID,
			MediaKind:      "document",
			MIMEType:       msg.Document.MimeType,
			Filename:       msg.Document.FileName,
			SizeBytes:      msg.Document.FileSize,
			Caption:        msg.Caption,
		})
	}

	if msg.Sticker != nil {
		attachments = append(attachments, InboundAttachment{
			Source:         "telegram",
			TelegramFileID: msg.Sticker.FileID,
			MediaKind:      "sticker",
			SizeBytes:      msg.Sticker.FileSize,
		})
	}

	return attachments
}

func (b *Bot) tgCall(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram %s error: %d %s", method, resp.StatusCode, string(respBody))
	}

	if out != nil {
		var res struct {
			OK          bool            `json:"ok"`
			Result      json.RawMessage `json:"result"`
			Description string          `json:"description"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return err
		}
		if !res.OK {
			return fmt.Errorf("telegram %s: %s", method, res.Description)
		}
		return json.Unmarshal(res.Result, out)
	}
	return nil
}

// telegramAllowedUpdates is the explicit update subscription. Limited to the update
// types the connector actually handles; extend as new tiers add handling.
var telegramAllowedUpdates = []string{"message", "edited_message", "channel_post", "callback_query"}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]tgUpdate, error) {
	params := map[string]any{
		"timeout":         30,
		"allowed_updates": telegramAllowedUpdates,
	}
	if offset > 0 {
		params["offset"] = offset
	}
	var updates []tgUpdate
	if err := b.tgCall(ctx, "getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (b *Bot) sendMessage(ctx context.Context, text string, replyMarkup any) {
	b.sendMessageForChat(ctx, b.cfg.OwnerChatID, text, replyMarkup)
}

func (b *Bot) sendMessageForChat(ctx context.Context, chatID int64, text string, replyMarkup any) {
	if chatID == 0 {
		chatID = b.cfg.OwnerChatID
	}
	if chatID == 0 {
		slog.Warn("telegram: sendMessageForChat called with chatID=0 and no ownerChatID; message dropped", "owner_chat_id", b.cfg.OwnerChatID)
		b.recordConnectorError(ctx, "", "send_message_missing_chat_id", "telegram sendMessageForChat called with chatID=0 and no ownerChatID", map[string]any{"owner_chat_id": b.cfg.OwnerChatID})
		return
	}
	slog.Debug("telegram: sending message to chat", "chat_id", chatID)
	_, _ = b.sendTextChunksForChat(ctx, chatID, text, replyMarkup, 0, 0, "")
}

func (b *Bot) sendChatAction(ctx context.Context, chatID int64, action string) error {
	return b.tgCall(ctx, "sendChatAction", map[string]any{
		"chat_id": chatID,
		"action":  action,
	}, nil)
}

func (b *Bot) sendMessageToChat(ctx context.Context, chatIDStr string, text string, replyMarkup any, replyToMessageID, messageThreadID int64, parseMode string) (int64, error) {
	var chatID int64
	var err error
	if chatID, err = strconv.ParseInt(chatIDStr, 10, 64); err != nil {
		return 0, err
	}
	if parseMode != "" && parseMode != "Markdown" && parseMode != "MarkdownV2" && parseMode != "HTML" {
		return 0, fmt.Errorf("invalid parse_mode: %s", parseMode)
	}
	text = sanitizeTelegramText(text)
	params := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if replyMarkup != nil {
		params["reply_markup"] = replyMarkup
	}
	if replyToMessageID != 0 {
		params["reply_to_message_id"] = replyToMessageID
	}
	if messageThreadID != 0 {
		params["message_thread_id"] = messageThreadID
	}
	if parseMode != "" {
		params["parse_mode"] = parseMode
	}

	var out struct {
		MessageID int64 `json:"message_id"`
	}
	if err := b.tgCall(ctx, "sendMessage", params, &out); err != nil {
		slog.Warn("telegram: sendMessage failed", "chat_id", chatID, "error", err)
		b.recordConnectorError(ctx, b.sessionIDForChat(chatID), "send_message_failed", err.Error(), map[string]any{
			"chat_id": chatID,
		})
		return 0, err
	}
	return out.MessageID, nil
}

func (b *Bot) editMessageForChat(ctx context.Context, chatID, messageID int64, text string) error {
	return b.editMessageForChatWithMarkup(ctx, chatID, messageID, text, nil, "")
}

// editRenderedForChat edits a message with markdown rendered to Telegram HTML,
// falling back to plain text if Telegram rejects the entities. Used to finalize
// streamed assistant replies (the streaming placeholder itself stays plain).
func (b *Bot) editRenderedForChat(ctx context.Context, chatID, messageID int64, text string) error {
	html := markdownToTelegramHTML(text)
	err := b.editMessageForChatWithMarkup(ctx, chatID, messageID, html, nil, "HTML")
	if err != nil && telegramHTMLParseError(err) {
		err = b.editMessageForChatWithMarkup(ctx, chatID, messageID, text, nil, "")
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified") {
		return nil
	}
	return err
}

func (b *Bot) editMessageForChatWithMarkup(ctx context.Context, chatID, messageID int64, text string, replyMarkup any, parseMode string) error {
	text = sanitizeTelegramText(text)
	params := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if replyMarkup != nil {
		params["reply_markup"] = replyMarkup
	}
	if parseMode != "" {
		params["parse_mode"] = parseMode
	}
	return b.tgCall(ctx, "editMessageText", params, nil)
}

func (b *Bot) replaceSessionPlaceholder(ctx context.Context, naviChatID string, chatID int64, content string, replyMarkup any) bool {
	stream, active := b.chatStreamSnapshot(naviChatID)
	if !active || stream.MessageID == 0 || stream.ChatID != chatID {
		return false
	}
	if err := b.editMessageForChatWithMarkup(ctx, chatID, stream.MessageID, content, replyMarkup, ""); err != nil {
		return false
	}
	b.clearSessionStream(naviChatID)
	return true
}

func sanitizeTelegramText(text string) string {
	return telegramBreakTagPattern.ReplaceAllString(text, "\n")
}

func (b *Bot) splitText(text string) []string {
	sanitized := sanitizeTelegramText(text)
	maxLen := b.MaxMessageLength()
	if maxLen <= 0 || len([]rune(sanitized)) <= maxLen {
		return []string{sanitized}
	}
	return intconnectors.SplitMessage(sanitized, maxLen)
}

func (b *Bot) sendTextChunksForChat(ctx context.Context, chatID int64, text string, replyMarkup any, replyToMessageID, messageThreadID int64, parseMode string) (int64, error) {
	chunks := b.splitText(text)
	var lastMessageID int64
	for idx, chunk := range chunks {
		chunkReplyMarkup := replyMarkup
		chunkReplyTo := replyToMessageID
		if idx > 0 {
			chunkReplyMarkup = nil
			chunkReplyTo = 0
		}
		messageID, err := b.sendChunkText(ctx, chatID, chunk, chunkReplyMarkup, chunkReplyTo, messageThreadID, parseMode)
		if err != nil {
			return 0, err
		}
		lastMessageID = messageID
	}
	return lastMessageID, nil
}

// sendChunkText sends one chunk. When parseMode is empty (the chat-reply default)
// it renders markdown to Telegram HTML and transparently falls back to plain text
// if Telegram rejects the entities, so a bad render never blocks delivery. An
// explicit parseMode is passed through unchanged.
func (b *Bot) sendChunkText(ctx context.Context, chatID int64, chunk string, replyMarkup any, replyToMessageID, messageThreadID int64, parseMode string) (int64, error) {
	chatIDStr := strconv.FormatInt(chatID, 10)
	if parseMode != "" {
		return b.sendMessageToChat(ctx, chatIDStr, chunk, replyMarkup, replyToMessageID, messageThreadID, parseMode)
	}
	html := markdownToTelegramHTML(chunk)
	id, err := b.sendMessageToChat(ctx, chatIDStr, html, replyMarkup, replyToMessageID, messageThreadID, "HTML")
	if err != nil && telegramHTMLParseError(err) {
		return b.sendMessageToChat(ctx, chatIDStr, chunk, replyMarkup, replyToMessageID, messageThreadID, "")
	}
	return id, err
}

// isUIShapingArtifact returns true if the content is a known UI shaping
// fallback that should not be delivered to external channels as a real message.
func isUIShapingArtifact(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	for _, phrase := range uiShapingPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

var uiShapingPhrases = []string{
	"i didn't generate a reply",
	"please try again or rephrase",
}

func (b *Bot) answerCallbackQuery(ctx context.Context, id, text string) error {
	params := map[string]any{
		"callback_query_id": id,
	}
	if text != "" {
		params["text"] = text
	}
	return b.tgCall(ctx, "answerCallbackQuery", params, nil)
}

func (b *Bot) renderSessionPartial(ctx context.Context, naviChatID string, chatID int64, content string) {
	if strings.TrimSpace(content) == "" || isUIShapingArtifact(content) {
		return
	}
	content = b.splitText(content)[0]
	b.stopSessionThinking(naviChatID)
	b.mu.Lock()
	stream := b.chatStream[naviChatID]
	b.mu.Unlock()
	if stream.MessageID == 0 || stream.ChatID != chatID {
		messageID, err := b.sendMessageToChat(ctx, strconv.FormatInt(chatID, 10), content, nil, 0, 0, "")
		if err != nil {
			log.Printf("telegram: send streaming partial failed: %v", err)
			return
		}
		b.mu.Lock()
		b.chatStream[naviChatID] = streamMessage{ChatID: chatID, MessageID: messageID, Content: content}
		b.mu.Unlock()
		return
	}
	if stream.Content == content {
		return
	}
	if err := b.editMessageForChat(ctx, chatID, stream.MessageID, content); err != nil {
		log.Printf("telegram: edit streaming partial failed: %v", err)
		return
	}
	b.mu.Lock()
	stream.Content = content
	b.chatStream[naviChatID] = stream
	b.mu.Unlock()
}

func (b *Bot) clearSessionStream(naviChatID string) {
	b.stopSessionThinking(naviChatID)
	b.mu.Lock()
	delete(b.chatStream, naviChatID)
	delete(b.chatRepair, naviChatID)
	delete(b.chatWatchdog, naviChatID)
	b.mu.Unlock()
}

func (b *Bot) chatStreamSnapshot(naviChatID string) (stream streamMessage, active bool) {
	if strings.TrimSpace(naviChatID) == "" {
		return streamMessage{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stream, ok := b.chatStream[naviChatID]
	return stream, ok && stream.MessageID != 0
}

func (b *Bot) beginSessionRepair(naviChatID string, placeholderMessageID int64) bool {
	if strings.TrimSpace(naviChatID) == "" || placeholderMessageID <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if current := b.chatRepair[naviChatID]; current == placeholderMessageID {
		return false
	}
	b.chatRepair[naviChatID] = placeholderMessageID
	return true
}

func (b *Bot) chatRepairSnapshot(naviChatID string) (int64, bool) {
	if strings.TrimSpace(naviChatID) == "" {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	placeholderMessageID, ok := b.chatRepair[naviChatID]
	return placeholderMessageID, ok && placeholderMessageID > 0
}

func (b *Bot) fetchSession(ctx context.Context, naviChatID string) (*navicore.ChatRuntimeView, error) {
	data, err := b.gatewayDo(ctx, http.MethodGet, "/api/navi/chats/"+naviChatID, nil)
	if err != nil {
		return nil, err
	}
	var entry navicore.ChatRuntimeView
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (b *Bot) fetchSessionRuntimeSummary(ctx context.Context, naviChatID string) (*sessionRuntimeSummary, error) {
	data, err := b.gatewayDo(ctx, http.MethodGet, "/api/navi/chats/"+naviChatID+"/runtime_summary", nil)
	if err != nil {
		return nil, err
	}
	var summary sessionRuntimeSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func sessionMessageByID(entry *navicore.ChatRuntimeView, messageID string) *navicore.ChatRuntimeMessage {
	if entry == nil {
		return nil
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	for i := range entry.Messages {
		msg := &entry.Messages[i]
		if msg.Role == "navi" && strings.TrimSpace(msg.ID) == messageID {
			return msg
		}
	}
	return nil
}

func latestAssistantMessageForRun(entry *navicore.ChatRuntimeView, runID string) *navicore.ChatRuntimeMessage {
	if entry == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	for i := len(entry.Messages) - 1; i >= 0; i-- {
		msg := &entry.Messages[i]
		if msg.Role == "navi" && strings.TrimSpace(msg.RunID) == runID && strings.TrimSpace(msg.Content) != "" {
			return msg
		}
	}
	return nil
}

func (b *Bot) scheduleCompletionRepair(ctx context.Context, naviChatID string, chatID, placeholderMessageID int64, finalMessageID string) {
	if !b.beginSessionRepair(naviChatID, placeholderMessageID) {
		return
	}
	go func() {
		timer := time.NewTimer(b.completionRepairGrace)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case <-timer.C:
		}

		repairedPlaceholderID, activeRepair := b.chatRepairSnapshot(naviChatID)
		stream, activeStream := b.chatStreamSnapshot(naviChatID)
		if !activeRepair || repairedPlaceholderID != placeholderMessageID || !activeStream || stream.MessageID != placeholderMessageID || stream.ChatID != chatID {
			return
		}

		const repairFallback = "The run completed, but the final reply was not delivered. Try /status or resend."

		repairText := repairFallback
		if strings.TrimSpace(finalMessageID) == "" {
			slog.Warn("telegram: run.completed repair missing final_message_id",
				"chat_id", naviChatID,
				"chat_id", chatID,
				"placeholder_message_id", placeholderMessageID,
			)
		} else {
			entry, err := b.fetchSession(context.Background(), naviChatID)
			if err != nil {
				slog.Warn("telegram: run.completed repair fetch failed",
					"chat_id", naviChatID,
					"chat_id", chatID,
					"placeholder_message_id", placeholderMessageID,
					"final_message_id", finalMessageID,
					"error", err,
				)
			} else if msg := sessionMessageByID(entry, finalMessageID); msg != nil && strings.TrimSpace(msg.Content) != "" {
				repairText = msg.Content
			} else {
				slog.Warn("telegram: run.completed repair message missing",
					"chat_id", naviChatID,
					"chat_id", chatID,
					"placeholder_message_id", placeholderMessageID,
					"final_message_id", finalMessageID,
				)
			}
		}

		stream, activeStream = b.chatStreamSnapshot(naviChatID)
		repairedPlaceholderID, activeRepair = b.chatRepairSnapshot(naviChatID)
		if !activeRepair || repairedPlaceholderID != placeholderMessageID || !activeStream || stream.MessageID != placeholderMessageID || stream.ChatID != chatID {
			return
		}
		slog.Info("telegram: repairing terminal completion from run.completed",
			"chat_id", naviChatID,
			"chat_id", chatID,
			"placeholder_message_id", placeholderMessageID,
			"final_message_id", strings.TrimSpace(finalMessageID),
			"content_len", len(repairText),
		)
		b.finalizeSessionMessage(context.Background(), naviChatID, chatID, repairText)
	}()
}

func (b *Bot) beginSessionWatchdog(naviChatID string) bool {
	if strings.TrimSpace(naviChatID) == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.chatWatchdog[naviChatID] {
		return false
	}
	b.chatWatchdog[naviChatID] = true
	return true
}

func terminalEventType(summary *sessionRuntimeSummary) string {
	if summary == nil || summary.TerminalEvent == nil {
		return ""
	}
	return strings.TrimSpace(summary.TerminalEvent.Type)
}

func (b *Bot) terminalRepairContent(ctx context.Context, naviChatID string, summary *sessionRuntimeSummary) (string, bool) {
	if summary == nil || summary.Run == nil {
		return "", false
	}
	status := strings.TrimSpace(summary.Run.Status)
	switch status {
	case string(schema.RunStatusCompleted):
		const repairFallback = "The run completed, but the final reply was not delivered. Try /status or resend."
		entry, err := b.fetchSession(ctx, naviChatID)
		if err != nil {
			return repairFallback, true
		}
		if summary.TerminalEvent != nil {
			if finalMessageID, _ := summary.TerminalEvent.Payload["final_message_id"].(string); strings.TrimSpace(finalMessageID) != "" {
				if msg := sessionMessageByID(entry, finalMessageID); msg != nil && strings.TrimSpace(msg.Content) != "" {
					return msg.Content, true
				}
			}
		}
		if msg := latestAssistantMessageForRun(entry, summary.Run.RunID); msg != nil {
			return msg.Content, true
		}
		return repairFallback, true
	case string(schema.RunStatusFailed):
		errorText := "The run failed."
		if summary.TerminalEvent != nil {
			if text, _ := summary.TerminalEvent.Payload["error"].(string); strings.TrimSpace(text) != "" {
				errorText = text
			}
		}
		return "NAVI run failed: " + errorText, true
	case string(schema.RunStatusCancelled):
		reason := "The run was cancelled."
		if summary.TerminalEvent != nil {
			if text, _ := summary.TerminalEvent.Payload["reason"].(string); strings.TrimSpace(text) != "" {
				reason = text
			}
		}
		return "NAVI run cancelled: " + reason, true
	default:
		return "", false
	}
}

func (b *Bot) startSessionTerminalWatchdog(naviChatID string, chatID int64) {
	if chatID == 0 || !b.beginSessionWatchdog(naviChatID) {
		return
	}
	watchdogStartedAt := time.Now()
	go func() {
		defer func() {
			b.mu.Lock()
			delete(b.chatWatchdog, naviChatID)
			b.mu.Unlock()
		}()
		ticker := time.NewTicker(b.terminalWatchdogPoll)
		defer ticker.Stop()
		deadline := time.NewTimer(defaultTerminalWatchdogTimeout)
		defer deadline.Stop()
		for {
			select {
			case <-b.done:
				return
			case <-deadline.C:
				stream, active := b.chatStreamSnapshot(naviChatID)
				if !active || stream.ChatID != chatID {
					return
				}
				slog.Warn("telegram: watchdog timed out waiting for run completion",
					"chat_id", naviChatID,
					"chat_id", chatID,
					"placeholder_message_id", stream.MessageID,
					"waited", defaultTerminalWatchdogTimeout,
				)
				b.finalizeSessionMessage(context.Background(), naviChatID, chatID,
					"NAVI took too long to respond. The run may still be processing — try /status or resend your message.")
				return
			case <-ticker.C:
			}

			stream, active := b.chatStreamSnapshot(naviChatID)
			if !active || stream.ChatID != chatID {
				return
			}

			summary, err := b.fetchSessionRuntimeSummary(context.Background(), naviChatID)
			if err != nil {
				continue
			}
			// Only act on runs that were updated AFTER this watchdog started.
			// A stale completed run from a prior session turn would have UpdatedAt
			// before watchdogStartedAt and must not be delivered as a repair.
			if summary.Run != nil && summary.Run.UpdatedAt.Before(watchdogStartedAt) {
				slog.Debug("telegram: watchdog skipping stale run",
					"chat_id", naviChatID,
					"run_id", summary.Run.RunID,
					"run_updated_at", summary.Run.UpdatedAt,
					"watchdog_started_at", watchdogStartedAt,
				)
				continue
			}
			repairText, terminal := b.terminalRepairContent(context.Background(), naviChatID, summary)
			if !terminal || strings.TrimSpace(repairText) == "" {
				continue
			}

			stream, active = b.chatStreamSnapshot(naviChatID)
			if !active || stream.ChatID != chatID {
				return
			}
			slog.Info("telegram: watchdog repairing unresolved placeholder",
				"chat_id", naviChatID,
				"chat_id", chatID,
				"placeholder_message_id", stream.MessageID,
				"run_status", strings.TrimSpace(summary.Run.Status),
				"terminal_event_type", terminalEventType(summary),
				"content_len", len(repairText),
			)
			b.finalizeSessionMessage(context.Background(), naviChatID, chatID, repairText)
			return
		}
	}()
}

func (b *Bot) finalizeSessionMessage(ctx context.Context, naviChatID string, chatID int64, content string) {
	b.stopSessionThinking(naviChatID)
	if strings.TrimSpace(content) == "" || isUIShapingArtifact(content) {
		slog.Debug("telegram: finalizeSessionMessage: dropping empty/ui-shaping content", "chat_id", naviChatID)
		b.clearSessionStream(naviChatID)
		return
	}
	b.mu.Lock()
	stream := b.chatStream[naviChatID]
	b.mu.Unlock()
	chunks := b.splitText(content)
	if stream.MessageID != 0 && stream.ChatID == chatID {
		// Always finalize via a rendered (HTML) edit: the streamed placeholder is
		// plain text, so even when the text is unchanged the markdown needs to be
		// rendered. editRenderedForChat tolerates "message is not modified".
		slog.Debug("telegram: finalizeSessionMessage: editing placeholder", "chat_id", naviChatID, "chat_id", chatID, "message_id", stream.MessageID)
		if err := b.editRenderedForChat(ctx, chatID, stream.MessageID, chunks[0]); err != nil {
			log.Printf("telegram: finalize streaming edit failed: %v", err)
			b.sendMessageForChat(ctx, chatID, content, nil)
			b.clearSessionStream(naviChatID)
			return
		}
		for _, chunk := range chunks[1:] {
			b.sendMessageForChat(ctx, chatID, chunk, nil)
		}
		b.clearSessionStream(naviChatID)
		return
	}
	slog.Debug("telegram: finalizeSessionMessage: sending new message", "chat_id", naviChatID, "chat_id", chatID, "content_len", len(content))
	b.sendMessageForChat(ctx, chatID, content, nil)
	b.clearSessionStream(naviChatID)
}

func (b *Bot) stopSessionThinking(naviChatID string) {
	if naviChatID == "" {
		return
	}
	b.mu.Lock()
	stop := b.chatTypingStop[naviChatID]
	delete(b.chatTypingStop, naviChatID)
	b.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// startSessionThinking signals that NAVI is working on a reply. It only emits
// Telegram's native "typing…" chat action — it deliberately does NOT post a
// "Thinking… 💭" placeholder message. The streamed/final reply is delivered as
// its own message by renderSessionPartial / finalizeSessionMessage.
func (b *Bot) startSessionThinking(ctx context.Context, naviChatID string, chatID int64) {
	if naviChatID == "" || chatID == 0 {
		return
	}
	b.stopSessionThinking(naviChatID)

	b.mu.Lock()
	delete(b.chatRepair, naviChatID)
	b.mu.Unlock()

	stop, err := b.StartTyping(ctx, strconv.FormatInt(chatID, 10))
	if err != nil {
		log.Printf("telegram: start typing failed: %v", err)
	} else if stop != nil {
		b.mu.Lock()
		b.chatTypingStop[naviChatID] = stop
		b.mu.Unlock()
	}
}

func mediaEndpointForContentType(contentType string) (endpoint, formField, defaultFilename string) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		return "sendPhoto", "photo", "photo"
	}
	return "sendDocument", "document", "document"
}

func (b *Bot) sendMediaPartToChat(ctx context.Context, chatIDStr string, part connectors.MediaPart) error {
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return err
	}
	if len(part.Data) == 0 {
		return fmt.Errorf("empty media payload")
	}
	endpoint, formField, defaultFilename := mediaEndpointForContentType(part.ContentType)
	filename := strings.TrimSpace(part.Filename)
	if filename == "" {
		filename = defaultFilename
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return err
	}
	fileWriter, err := writer.CreateFormFile(formField, filename)
	if err != nil {
		return err
	}
	if _, err := fileWriter.Write(part.Data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/"+endpoint, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram %s: %d %s", endpoint, resp.StatusCode, string(respBody))
	}
	return nil
}

func (b *Bot) isAllowedChat(chatID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.allowedChats[chatID]
}

func (b *Bot) allowChat(chatID int64) {
	if chatID == 0 {
		return
	}
	b.mu.Lock()
	b.allowedChats[chatID] = true
	b.mu.Unlock()
}

func pairingCommand(text string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return "", false
	}
	cmd := strings.ToLower(parts[0])
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if cmd != "/pair" {
		return "", false
	}
	if len(parts) < 2 {
		return "", true
	}
	return parts[1], true
}

func parseModelCommand(text string) modelCommand {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return modelCommand{}
	}
	cmd := strings.ToLower(parts[0])
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if cmd != "/model" {
		return modelCommand{}
	}
	if len(parts) == 1 {
		return modelCommand{
			Handled: true,
			Usage:   "Usage: /model list | /model current | /model set <provider> <model> | /model <provider> <model>",
		}
	}

	switch strings.ToLower(parts[1]) {
	case "list":
		if len(parts) != 2 {
			return modelCommand{
				Handled: true,
				Usage:   "Usage: /model list",
			}
		}
		return modelCommand{Handled: true, Action: "list"}
	case "current", "active", "status":
		if len(parts) != 2 {
			return modelCommand{
				Handled: true,
				Usage:   "Usage: /model current",
			}
		}
		return modelCommand{Handled: true, Action: "current"}
	case "set", "use":
		if len(parts) < 4 {
			return modelCommand{
				Handled: true,
				Usage:   "Usage: /model set <provider> <model>",
			}
		}
		return modelCommand{
			Handled:  true,
			Action:   "set",
			Provider: strings.ToLower(strings.TrimSpace(parts[2])),
			Model:    strings.TrimSpace(strings.Join(parts[3:], " ")),
		}
	default:
		if len(parts) < 3 {
			return modelCommand{
				Handled: true,
				Usage:   "Usage: /model <provider> <model>",
			}
		}
		return modelCommand{
			Handled:  true,
			Action:   "set",
			Provider: strings.ToLower(strings.TrimSpace(parts[1])),
			Model:    strings.TrimSpace(strings.Join(parts[2:], " ")),
		}
	}
}

func formatTelegramLLMCatalog(catalog telegramLLMCatalog) string {
	if len(catalog.Providers) == 0 {
		return "No providers configured."
	}
	var b strings.Builder
	b.WriteString("Available providers and models:\n")
	for _, provider := range catalog.Providers {
		name := strings.TrimSpace(provider.DisplayName)
		if name == "" {
			name = strings.TrimSpace(provider.Key)
		}
		if name == "" {
			continue
		}
		b.WriteString("\n" + name + "\n")
		for _, model := range provider.Models {
			modelName := strings.TrimSpace(model.Name)
			if modelName == "" {
				continue
			}
			b.WriteString("- " + modelName + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func (b *Bot) handleModelCommand(ctx context.Context, text string, sendReply func(string, any)) bool {
	cmd := parseModelCommand(text)
	if !cmd.Handled {
		return false
	}
	if cmd.Usage != "" {
		sendReply(cmd.Usage, nil)
		return true
	}

	switch cmd.Action {
	case "list":
		data, err := b.gatewayDo(ctx, http.MethodGet, "/api/llm/catalog", nil)
		if err != nil {
			sendReply("Model catalog failed: "+err.Error(), nil)
			return true
		}
		var catalog telegramLLMCatalog
		if err := json.Unmarshal(data, &catalog); err != nil {
			sendReply("Model catalog failed: "+err.Error(), nil)
			return true
		}
		sendReply(formatTelegramLLMCatalog(catalog), nil)
		return true
	case "current":
		data, err := b.gatewayDo(ctx, http.MethodGet, "/api/llm/active", nil)
		if err != nil {
			sendReply("Model status failed: "+err.Error(), nil)
			return true
		}
		var active struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := json.Unmarshal(data, &active); err != nil {
			sendReply("Model status failed: "+err.Error(), nil)
			return true
		}
		sendReply(fmt.Sprintf("Active provider: %s\nActive model: %s", active.Provider, active.Model), nil)
		return true
	case "set":
		_, err := b.gatewayDo(ctx, http.MethodPut, "/api/llm/active", map[string]string{
			"provider": cmd.Provider,
			"model":    cmd.Model,
		})
		if err != nil {
			sendReply("Model switch failed: "+err.Error(), nil)
			return true
		}
		sendReply(fmt.Sprintf("Switched to provider %s, model %s.", cmd.Provider, cmd.Model), nil)
		return true
	default:
		sendReply("Usage: /model list | /model current | /model set <provider> <model> | /model <provider> <model>", nil)
		return true
	}
}

func (b *Bot) handlePairing(ctx context.Context, chatID int64, text string) bool {
	code, isPairCommand := pairingCommand(text)
	if !isPairCommand {
		return false
	}
	if strings.TrimSpace(b.cfg.PairingCode) == "" {
		b.sendMessageForChat(ctx, chatID, "Pairing is disabled for this bot.", nil)
		return true
	}
	if code == "" {
		b.sendMessageForChat(ctx, chatID, "Usage: /pair <code>", nil)
		return true
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(b.cfg.PairingCode)) != 1 {
		b.sendMessageForChat(ctx, chatID, "Pairing failed: invalid code.", nil)
		return true
	}
	b.allowChat(chatID)
	b.sendMessageForChat(ctx, chatID, "Pairing successful. This chat is now authorized.", nil)
	if b.cfg.OwnerChatID != 0 && b.cfg.OwnerChatID != chatID {
		b.sendMessageForChat(ctx, b.cfg.OwnerChatID, fmt.Sprintf("Telegram chat %d paired successfully.", chatID), nil)
	}
	return true
}

func (b *Bot) handleCommand(ctx context.Context, chatID int64, text string, sendReply func(text string, replyMarkup any)) bool {
	parts := strings.SplitN(text, " ", 2)
	cmd := parts[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	switch cmd {
	case "/navi":
		if args != "" {
			sendReply("Usage: /navi", nil)
			return true
		}
		naviChatID, err := b.ensureChat(ctx, chatID)
		if err != nil {
			sendReply("NAVI Initialization Error: "+err.Error(), nil)
			return true
		}
		b.clearDirectiveFocus(chatID)
		sendReply("NAVI session started.", nil)
		log.Printf("telegram: focus switched to NAVI session %s", naviChatID)
		return true

	case "/new":
		if args == "" {
			sendReply("Usage: /new <title>", nil)
			return true
		}
		data, err := b.gatewayDo(ctx, "POST", "/api/directives", map[string]string{"title": args, "mode": "ADVISE"})
		if err != nil {
			sendReply("Error: "+err.Error(), nil)
			return true
		}
		var d struct {
			DirectiveID string `json:"directive_id"`
		}
		json.Unmarshal(data, &d)
		b.bindDirectiveToChat(chatID, d.DirectiveID)
		sendReply("Directive created: "+d.DirectiveID+"\nIt is now the active directive.", nil)
		log.Printf("telegram: focus switched to Orchestrator directive %s", d.DirectiveID)
		return true

	case "/use":
		if args == "" {
			sendReply("Usage: /use <directive_id>", nil)
			return true
		}
		b.bindDirectiveToChat(chatID, args)
		sendReply("Active directive set to: "+args, nil)
		return true

	case "/list":
		data, err := b.gatewayDo(ctx, "GET", "/api/directives", nil)
		if err != nil {
			sendReply("Error: "+err.Error(), nil)
			return true
		}
		var list []map[string]any
		json.Unmarshal(data, &list)
		if len(list) == 0 {
			sendReply("No directives found.", nil)
			return true
		}

		limit := 5
		if len(list) < 5 {
			limit = len(list)
		}

		var sb strings.Builder
		sb.WriteString("Recent Directives:\n")
		for i := 0; i < limit; i++ {
			d := list[i]
			id := d["directive_id"].(string)
			status := d["status"].(string)
			sb.WriteString(fmt.Sprintf("%d. %s [%s]\n", i+1, id, status))
		}

		sendReply(sb.String(), nil)
		return true

	case "/implement":
		if args == "" {
			sendReply("Usage: /implement <directive_id>", nil)
			return true
		}
		_, err := b.gatewayDo(ctx, "PUT", "/api/directives/"+args+"/mode", map[string]string{"mode": "ACT"})
		if err != nil {
			sendReply("Error: "+err.Error(), nil)
			return true
		}
		sendReply("Mode set to ACT.", nil)
		return true

	case "/status":
		b.handleStatusCommand(ctx, sendReply)
		return true

	case "/hitl":
		data, err := b.gatewayDo(ctx, "GET", "/api/hitl", nil)
		if err != nil {
			sendReply("Error: "+err.Error(), nil)
			return true
		}
		var hitls []map[string]any
		json.Unmarshal(data, &hitls)
		if len(hitls) == 0 {
			sendReply("No pending HITL approvals.", nil)
			return true
		}
		for _, h := range hitls {
			eid := h["event_id"].(string)
			kb := map[string]any{
				"inline_keyboard": [][]map[string]string{
					{
						{"text": "Approve", "callback_data": "approve_" + eid},
						{"text": "Reject", "callback_data": "reject_" + eid},
					},
				},
			}
			msg := fmt.Sprintf("Task %v - Reason: %v", h["task_id"], h["reason"])
			sendReply(msg, kb)
		}
		return true

	case "/pair":
		sendReply("This chat is already authorized.", nil)
		return true

	case "/start":
		_, err := b.ensureChat(ctx, chatID)
		if err != nil {
			sendReply("NAVI Initialization Error: "+err.Error(), nil)
			return true
		}
		return true

	default:
		if strings.HasPrefix(cmd, "/") {
			sendReply("Unknown command. Try /navi, /new, /use, /list, /implement, /status, /model, /hitl, /pair, or /start.", nil)
			return true
		}
	}
	return false
}

func isGroupChatType(chatType string) bool {
	switch strings.TrimSpace(strings.ToLower(chatType)) {
	case "group", "supergroup":
		return true
	default:
		return false
	}
}

// resolveGroupConfig returns the effective group config for a chat, preferring a
// specific chat-ID entry over the "*" wildcard. A zero value means "no config".
func (b *Bot) resolveGroupConfig(chatID int64) GroupConfig {
	b.mu.Lock()
	groups := b.cfg.Groups
	b.mu.Unlock()
	if len(groups) == 0 {
		return GroupConfig{}
	}
	if g, ok := groups[strconv.FormatInt(chatID, 10)]; ok {
		return g
	}
	if g, ok := groups["*"]; ok {
		return g
	}
	return GroupConfig{}
}

// messageMentionsBot reports whether the message @-mentions this bot or replies to
// one of the bot's own messages (implicit mention).
func (b *Bot) messageMentionsBot(msg *tgMessage) bool {
	if msg == nil {
		return false
	}
	b.mu.Lock()
	username := b.botUsername
	b.mu.Unlock()

	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.IsBot {
		if username == "" || strings.EqualFold(strings.TrimSpace(msg.ReplyToMessage.From.Username), username) {
			return true
		}
	}
	if username == "" {
		return false
	}
	needle := "@" + strings.ToLower(username)
	haystack := strings.ToLower(msg.Text + " " + msg.Caption)
	return strings.Contains(haystack, needle)
}

// shouldProcessGroupMessage decides whether a group message should be handled.
// DMs are always processed. Groups require a mention unless require_mention is
// explicitly disabled for that group. Disabled groups are skipped entirely.
func (b *Bot) shouldProcessGroupMessage(msg *tgMessage) bool {
	if msg == nil {
		return false
	}
	if !isGroupChatType(msg.Chat.Type) {
		return true
	}
	cfg := b.resolveGroupConfig(msg.Chat.ID)
	if cfg.Enabled != nil && !*cfg.Enabled {
		return false
	}
	requireMention := true
	if cfg.RequireMention != nil {
		requireMention = *cfg.RequireMention
	}
	if !requireMention {
		return true
	}
	return b.messageMentionsBot(msg)
}

func (b *Bot) isGroupDisabled(msg *tgMessage) bool {
	if msg == nil || !isGroupChatType(msg.Chat.Type) {
		return false
	}
	cfg := b.resolveGroupConfig(msg.Chat.ID)
	return cfg.Enabled != nil && !*cfg.Enabled
}

func telegramSenderName(user *tgUser) string {
	if user == nil {
		return "user"
	}
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if name != "" {
		return name
	}
	if u := strings.TrimSpace(user.Username); u != "" {
		return u
	}
	return "user"
}

func telegramChatDisplayName(chat tgChat) string {
	if title := strings.TrimSpace(chat.Title); title != "" {
		return title
	}
	if username := strings.TrimSpace(chat.Username); username != "" {
		return username
	}
	name := strings.TrimSpace(strings.TrimSpace(chat.FirstName) + " " + strings.TrimSpace(chat.LastName))
	if name != "" {
		return name
	}
	return strconv.FormatInt(chat.ID, 10)
}

func truncateForEnvelope(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	const max = 200
	runes := []rune(text)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return text
}

func forwardOriginLabel(origin *tgForwardOrigin) string {
	if origin == nil {
		return ""
	}
	switch {
	case origin.SenderUser != nil:
		return telegramSenderName(origin.SenderUser)
	case strings.TrimSpace(origin.SenderUserName) != "":
		return strings.TrimSpace(origin.SenderUserName)
	case origin.SenderChat != nil && strings.TrimSpace(origin.SenderChat.Title) != "":
		return strings.TrimSpace(origin.SenderChat.Title)
	case origin.Chat != nil && strings.TrimSpace(origin.Chat.Title) != "":
		return strings.TrimSpace(origin.Chat.Title)
	default:
		return "unknown source"
	}
}

// describeAttachments returns a short bracketed note acknowledging inbound media so
// the agent is aware of it, even though raw media bytes are not yet ingested (see
// T1-5 in docs/specs/telegram-upgrades.md). Returns "" when there is no media.
func describeAttachments(attachments []InboundAttachment) string {
	if len(attachments) == 0 {
		return ""
	}
	notes := make([]string, 0, len(attachments))
	for _, a := range attachments {
		switch a.MediaKind {
		case "photo":
			notes = append(notes, "[User attached a photo]")
		case "voice":
			notes = append(notes, "[User sent a voice message]")
		case "audio":
			if a.Filename != "" {
				notes = append(notes, "[User attached an audio file: "+a.Filename+"]")
			} else {
				notes = append(notes, "[User attached an audio file]")
			}
		case "video":
			notes = append(notes, "[User attached a video]")
		case "document":
			if a.Filename != "" {
				notes = append(notes, "[User attached a document: "+a.Filename+"]")
			} else {
				notes = append(notes, "[User attached a document]")
			}
		case "sticker":
			notes = append(notes, "[User sent a sticker]")
		default:
			notes = append(notes, "[User attached media]")
		}
	}
	return strings.Join(notes, "\n")
}

// buildInboundEnvelope wraps group messages with sender attribution and reply/forward
// context so the agent can tell speakers and referents apart. DMs are returned as-is.
func buildInboundEnvelope(msg *tgMessage, text string, isGroup bool) string {
	if msg == nil || !isGroup {
		return text
	}
	var lines []string

	header := "[From " + telegramSenderName(msg.From)
	if msg.From != nil {
		if u := strings.TrimSpace(msg.From.Username); u != "" {
			header += " (@" + u + ")"
		}
	}
	if title := strings.TrimSpace(msg.Chat.Title); title != "" {
		header += " in \"" + title + "\""
	}
	header += "]"
	lines = append(lines, header)

	if reply := msg.ReplyToMessage; reply != nil {
		replyBody := strings.TrimSpace(reply.Text)
		if replyBody == "" {
			replyBody = strings.TrimSpace(reply.Caption)
		}
		line := "[Replying to " + telegramSenderName(reply.From) + "]"
		if replyBody != "" {
			line += " \"" + truncateForEnvelope(replyBody) + "\""
		}
		lines = append(lines, line)
	}

	if label := forwardOriginLabel(msg.ForwardOrigin); label != "" {
		lines = append(lines, "[Forwarded from "+label+"]")
	}

	lines = append(lines, text)
	return strings.Join(lines, "\n")
}

func (b *Bot) handleUpdate(ctx context.Context, u tgUpdate) {
	if u.CallbackQuery != nil && u.CallbackQuery.Message != nil {
		chatID := u.CallbackQuery.Message.Chat.ID
		if b.isAllowedChat(chatID) {
			b.handleCallback(ctx, chatID, u.CallbackQuery)
		}
		return
	}

	msg := u.Message
	if msg == nil {
		msg = u.EditedMessage
	}
	if msg == nil {
		msg = u.ChannelPost
	}
	if msg == nil {
		return
	}

	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}

	if !b.isAllowedChat(chatID) {
		_ = b.handlePairing(ctx, chatID, text)
		return
	}

	if b.isGroupDisabled(msg) {
		return
	}

	// Phase 1: Parse media metadata but don't ingest yet
	attachments := b.parseMediaMetadata(msg)

	sendReply := func(text string, replyMarkup any) {
		b.sendMessageForChat(ctx, chatID, text, replyMarkup)
	}

	if text != "" {
		if b.handleModelCommand(ctx, text, sendReply) {
			return
		}

		if strings.HasPrefix(text, "/") {
			if b.handleCommand(ctx, chatID, text, sendReply) {
				return
			}
		}
	}

	if text == "" && len(attachments) == 0 {
		return
	}

	// In groups, only respond when mentioned (unless require_mention is disabled).
	// Commands above always process; this gates free-form messages only.
	if !b.shouldProcessGroupMessage(msg) {
		return
	}

	// CIP P1: emit an IntakeRecord for every processed inbound message alongside
	// existing handling. Emission is best-effort; failure is logged but never
	// interrupts message delivery.
	if b.emitIntake != nil {
		if data := b.buildIntakeRecord(msg); data != nil {
			if err := b.emitIntake(ctx, data); err != nil {
				slog.Warn("telegram: intake emit failed", "connector", b.name, "err", err)
			}
		}
	}

	isGroup := isGroupChatType(msg.Chat.Type)
	bodyText := text
	if note := describeAttachments(attachments); note != "" {
		if bodyText == "" {
			bodyText = note
		} else {
			bodyText = note + "\n" + bodyText
		}
	}
	content := buildInboundEnvelope(msg, bodyText, isGroup)

	b.mu.Lock()
	dirID := b.directiveFocusForChatLocked(chatID)
	b.mu.Unlock()

	if dirID != "" {
		b.bindDirectiveToChat(chatID, dirID)
		_, err := b.gatewayDo(ctx, "POST", "/api/directives/"+dirID+"/message", map[string]string{"content": content})
		if err != nil {
			sendReply("Failed to post message to directive: "+err.Error(), nil)
		}
		return
	}

	resolved, err := b.resolveEndpointForMessage(ctx, msg)
	if err != nil {
		sendReply("NAVI Initialization Error: "+err.Error(), nil)
		return
	}
	naviChatID := resolved.ChatID
	originEndpointID := resolved.EndpointID

	sourceRef := telegramMessageRef(msg)
	idempotencyKey := telegramMessageIdempotencyKey(msg)
	b.startSessionThinking(ctx, naviChatID, chatID)
	queueRes, err := b.queueSessionMessage(ctx, naviChatID, content, sourceRef, idempotencyKey, originEndpointID)
	if err != nil {
		b.finalizeSessionMessage(ctx, naviChatID, chatID, "NAVI Communication Error: "+err.Error())
		return
	}
	if notice, replyMarkup, ok := queuedMessageDeferredNotice(queueRes); ok {
		if !b.replaceSessionPlaceholder(ctx, naviChatID, chatID, notice, replyMarkup) {
			b.clearSessionStream(naviChatID)
			b.sendMessageForChat(ctx, chatID, notice, replyMarkup)
		}
		return
	}
	if !queuedMessageStartsThinking(queueRes) {
		b.clearSessionStream(naviChatID)
	}
}

func (b *Bot) handleCallback(ctx context.Context, chatID int64, cb *tgCallbackQuery) {
	defer b.answerCallbackQuery(ctx, cb.ID, "")
	sendReply := func(text string, replyMarkup any) {
		b.sendMessageForChat(ctx, chatID, text, replyMarkup)
	}
	if strings.HasPrefix(cb.Data, "proposal_approve:") {
		proposalID := strings.TrimPrefix(cb.Data, "proposal_approve:")
		_, err := b.gatewayDo(ctx, "POST", "/api/proposals/"+proposalID+"/resolve", map[string]string{"action": "approve"})
		if err != nil {
			sendReply("Proposal approval failed: "+err.Error(), nil)
			return
		}
		sendReply("Proposal approved.", nil)
	} else if strings.HasPrefix(cb.Data, "proposal_reject:") {
		proposalID := strings.TrimPrefix(cb.Data, "proposal_reject:")
		_, err := b.gatewayDo(ctx, "POST", "/api/proposals/"+proposalID+"/resolve", map[string]string{"action": "decline"})
		if err != nil {
			sendReply("Proposal rejection failed: "+err.Error(), nil)
			return
		}
		sendReply("Proposal rejected.", nil)
	} else if strings.HasPrefix(cb.Data, "approve_") {
		eid := strings.TrimPrefix(cb.Data, "approve_")
		_, err := b.gatewayDo(ctx, "POST", "/api/hitl/"+eid+"/approve", nil)
		if err != nil {
			sendReply("Approval failed: "+err.Error(), nil)
			return
		}
		sendReply("HITL Approved.", nil)
	} else if strings.HasPrefix(cb.Data, "reject_") {
		eid := strings.TrimPrefix(cb.Data, "reject_")
		_, err := b.gatewayDo(ctx, "POST", "/api/hitl/"+eid+"/reject", nil)
		if err != nil {
			sendReply("Rejection failed: "+err.Error(), nil)
			return
		}
		sendReply("HITL Rejected.", nil)
	}
}

func stringListFromPayload(payload map[string]any, key string) []string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return nil
	}
	switch values := raw.(type) {
	case []string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				out = append(out, value)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func stringFromPayload(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func formatProposalWaitingText(proposalID, toolName, reason string, payload map[string]any) string {
	lines := []string{"Approval needed."}
	if summary := stringFromPayload(payload, "proposal_summary"); summary != "" {
		lines = append(lines, "Action: "+summary)
	}
	if capability := stringFromPayload(payload, "capability_description"); capability != "" {
		lines = append(lines, "Capability: "+capability)
	}
	if sideEffects := stringListFromPayload(payload, "side_effects"); len(sideEffects) > 0 {
		lines = append(lines, "Side effects: "+strings.Join(sideEffects, ", "))
	}
	if riskTier := stringFromPayload(payload, "risk_tier"); riskTier != "" {
		lines = append(lines, "Risk: "+riskTier)
	}
	if proposalID != "" {
		lines = append(lines, "Proposal: "+proposalID)
	}
	if toolName != "" {
		lines = append(lines, "Tool: "+toolName)
	}
	if reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	return strings.Join(lines, "\n")
}

func proposalReplyMarkup(proposalID string) any {
	if strings.TrimSpace(proposalID) == "" {
		return nil
	}
	return map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{"text": "Approve", "callback_data": "proposal_approve:" + proposalID},
				{"text": "Reject", "callback_data": "proposal_reject:" + proposalID},
			},
		},
	}
}

// fetchAndNotifyHITL fetches pending HITL items from the gateway and sends Telegram
// messages for any not yet notified. Used by hitlPoller and testable with a mock gateway.
func (b *Bot) fetchAndNotifyHITL(ctx context.Context) {
	data, err := b.gatewayDo(ctx, "GET", "/api/hitl", nil)
	if err != nil {
		return
	}
	var hitls []map[string]any
	if err := json.Unmarshal(data, &hitls); err != nil {
		return
	}
	for _, h := range hitls {
		eid, _ := h["event_id"].(string)
		if eid == "" {
			continue
		}

		b.mu.Lock()
		notified := b.notifiedHITL[eid]
		if !notified {
			b.notifiedHITL[eid] = true
		}
		b.mu.Unlock()

		if !notified {
			kb := map[string]any{
				"inline_keyboard": [][]map[string]string{
					{
						{"text": "Approve", "callback_data": "approve_" + eid},
						{"text": "Reject", "callback_data": "reject_" + eid},
					},
				},
			}
			msg := fmt.Sprintf("Task %v requires input - Reason: %v", h["task_id"], h["reason"])
			b.sendMessage(ctx, msg, kb)
		}
	}
}

func (b *Bot) hitlPoller(ctx context.Context) {
	tickSource := b.newHITLTickSource
	if tickSource == nil {
		tickSource = func() (<-chan time.Time, func()) {
			ticker := time.NewTicker(60 * time.Second)
			return ticker.C, ticker.Stop
		}
	}
	tickCh, stop := tickSource()
	if stop != nil {
		defer stop()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case <-tickCh:
			b.fetchAndNotifyHITL(ctx)
		}
	}
}

func (b *Bot) wsLivePoller(ctx context.Context) {
	workers := make(map[string]context.CancelFunc)
	reconcile := func() {
		desired := b.liveSessionSnapshot()
		want := make(map[string]struct{}, len(desired))
		for _, naviChatID := range desired {
			want[naviChatID] = struct{}{}
			if workers[naviChatID] != nil {
				continue
			}
			workerCtx, cancel := context.WithCancel(ctx)
			workers[naviChatID] = cancel
			go b.sessionLiveLoop(workerCtx, naviChatID)
		}
		for naviChatID, cancel := range workers {
			if _, ok := want[naviChatID]; ok {
				continue
			}
			cancel()
			delete(workers, naviChatID)
		}
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		for _, cancel := range workers {
			cancel()
		}
	}()

	reconcile()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func (b *Bot) sessionLiveLoop(ctx context.Context, naviChatID string) {
	wsURL := strings.Replace(b.cfg.GatewayURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += "/ws/live"

	var (
		lastSeq       int64
		connectedOnce bool
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		default:
		}

		tok, chatID, isFresh := b.chatStreamState(naviChatID)
		if tok == "" || chatID == 0 {
			if !b.waitForLiveRetry(ctx, 2*time.Second) {
				return
			}
			continue
		}

		header := http.Header{}
		header.Set("X-API-Key", tok)
		header.Set("X-Sub-Navi", "true")

		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
		if err != nil {
			slog.Warn("telegram: ws live dial error, retrying", "error", err, "wsURL", wsURL, "chat_id", naviChatID)
			b.recordConnectorError(ctx, naviChatID, "live_stream_dial_failed", err.Error(), map[string]any{
				"ws_url":           wsURL,
				"chat_id":          naviChatID,
				"telegram_chat_id": chatID,
			})
			if !b.waitForLiveRetry(ctx, 5*time.Second) {
				return
			}
			continue
		}

		connectParams := map[string]any{
			"chat_id":   naviChatID,
			"after_seq": lastSeq,
			"stream":    "user",
		}
		replayMode := "reconnect"
		if lastSeq == 0 {
			switch {
			case !connectedOnce && isFresh:
				replayMode = "fresh"
				connectParams["replay"] = false
			case !connectedOnce:
				replayMode = "recovered"
				connectParams["replay"] = false
			default:
				replayMode = "reconnect_zero_seq"
			}
		}

		connectFrame, _ := json.Marshal(map[string]any{
			"type":   "req",
			"id":     "connect",
			"method": "connect",
			"params": connectParams,
		})
		if err := conn.WriteMessage(websocket.TextMessage, connectFrame); err != nil {
			slog.Warn("telegram: ws live init error", "error", err, "chat_id", naviChatID)
			b.recordConnectorError(ctx, naviChatID, "live_stream_init_failed", err.Error(), map[string]any{
				"ws_url":      wsURL,
				"chat_id":     naviChatID,
				"after_seq":   lastSeq,
				"replay_mode": replayMode,
			})
			conn.Close()
			if !b.waitForLiveRetry(ctx, 5*time.Second) {
				return
			}
			continue
		}

		cursorSeq, err := readLiveConnectAck(conn)
		if err != nil {
			slog.Warn("telegram: ws live ack error", "error", err, "chat_id", naviChatID)
			b.recordConnectorError(ctx, naviChatID, "live_stream_ack_failed", err.Error(), map[string]any{
				"ws_url":      wsURL,
				"chat_id":     naviChatID,
				"after_seq":   lastSeq,
				"replay_mode": replayMode,
			})
			conn.Close()
			if !b.waitForLiveRetry(ctx, 5*time.Second) {
				return
			}
			continue
		}
		if cursorSeq > lastSeq {
			lastSeq = cursorSeq
		}
		connectedOnce = true
		if isFresh {
			b.clearFreshSession(naviChatID)
		}

		slog.Info("telegram: connected to /ws/live", "chat_id", naviChatID, "after_seq", lastSeq, "replay_mode", replayMode)

		connDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-b.done:
				_ = conn.Close()
			case <-connDone:
			}
		}()

		for {
			chatID = b.chatIDForSession(naviChatID)
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				close(connDone)
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					slog.Debug("telegram: ws read deadline, reconnecting", "chat_id", naviChatID)
				} else {
					slog.Warn("telegram: ws read error", "error", err, "chat_id", naviChatID)
					b.recordConnectorError(ctx, naviChatID, "live_stream_read_failed", err.Error(), map[string]any{
						"chat_id":          naviChatID,
						"telegram_chat_id": chatID,
						"last_seq":         lastSeq,
					})
				}
				_ = conn.Close()
				break
			}

			var frame struct {
				Type  string `json:"type"`
				Event struct {
					Type    string         `json:"type"`
					Seq     int64          `json:"seq"`
					Payload map[string]any `json:"payload"`
				} `json:"event"`
			}
			if err := json.Unmarshal(msg, &frame); err != nil {
				continue
			}
			if frame.Type != "event" {
				continue
			}
			if frame.Event.Seq > 0 {
				lastSeq = frame.Event.Seq
			}
			slog.Debug("telegram: ws event received", "event_type", frame.Event.Type, "seq", frame.Event.Seq, "chat_id", naviChatID, "chat_id", chatID)
			switch frame.Event.Type {
			case "assistant.message.partial":
				if isInternalRuntimeChatID(naviChatID) {
					continue
				}
				content, _ := frame.Event.Payload["content"].(string)
				if content == "" {
					continue
				}
				if chatID != 0 {
					b.renderSessionPartial(ctx, naviChatID, chatID, content)
				} else {
					b.sendMessage(ctx, content, nil)
				}
			case "assistant.message.completed":
				if isInternalRuntimeChatID(naviChatID) {
					continue
				}
				stream, active := b.chatStreamSnapshot(naviChatID)
				slog.Debug("telegram: terminal live event received", "event_type", frame.Event.Type, "seq", frame.Event.Seq, "chat_id", naviChatID, "chat_id", chatID, "placeholder_active", active, "placeholder_message_id", stream.MessageID)
				if frame.Event.Payload["message_kind"] == string(schema.AssistantMessageKindProactive) {
					content, _ := frame.Event.Payload["content"].(string)
					content = strings.TrimSpace(content)
					if content == "" {
						continue
					}
					text := "Proactive update:\n" + content
					if chatID != 0 {
						b.sendMessageForChat(ctx, chatID, text, nil)
					} else {
						b.sendMessage(ctx, text, nil)
					}
					continue
				}
				content, _ := frame.Event.Payload["content"].(string)
				if content == "" {
					continue
				}
				if chatID != 0 {
					slog.Debug("telegram: handling terminal live event", "event_type", frame.Event.Type, "chat_id", naviChatID, "chat_id", chatID, "content_len", len(content))
					b.finalizeSessionMessage(ctx, naviChatID, chatID, content)
				} else {
					b.sendMessage(ctx, content, nil)
				}
			case "proposal.waiting":
				b.stopSessionThinking(naviChatID)
				b.clearSessionStream(naviChatID)
				proposalID, _ := frame.Event.Payload["proposal_id"].(string)
				toolName, _ := frame.Event.Payload["tool_name"].(string)
				reason, _ := frame.Event.Payload["reason"].(string)
				text := formatProposalWaitingText(proposalID, toolName, reason, frame.Event.Payload)
				replyMarkup := proposalReplyMarkup(proposalID)
				if chatID != 0 {
					b.sendMessageForChat(ctx, chatID, text, replyMarkup)
				} else {
					b.sendMessage(ctx, text, replyMarkup)
				}
			case "run.failed":
				errText, _ := frame.Event.Payload["error"].(string)
				if errText == "" {
					errText = "The run failed."
				}
				stream, active := b.chatStreamSnapshot(naviChatID)
				slog.Warn("telegram: terminal live event received", "event_type", frame.Event.Type, "seq", frame.Event.Seq, "chat_id", naviChatID, "chat_id", chatID, "placeholder_active", active, "placeholder_message_id", stream.MessageID, "error", errText)
				if chatID != 0 {
					slog.Debug("telegram: handling terminal live event", "event_type", frame.Event.Type, "chat_id", naviChatID, "chat_id", chatID, "content_len", len(errText))
					b.finalizeSessionMessage(ctx, naviChatID, chatID, "NAVI run failed: "+errText)
				} else {
					b.sendMessage(ctx, "NAVI run failed: "+errText, nil)
				}
			case "run.cancelled":
				reason, _ := frame.Event.Payload["reason"].(string)
				if reason == "" {
					reason = "The run was cancelled."
				}
				stream, active := b.chatStreamSnapshot(naviChatID)
				slog.Info("telegram: terminal live event received", "event_type", frame.Event.Type, "seq", frame.Event.Seq, "chat_id", naviChatID, "chat_id", chatID, "placeholder_active", active, "placeholder_message_id", stream.MessageID, "reason", reason)
				if chatID != 0 {
					slog.Debug("telegram: handling terminal live event", "event_type", frame.Event.Type, "chat_id", naviChatID, "chat_id", chatID, "content_len", len(reason))
					b.finalizeSessionMessage(ctx, naviChatID, chatID, "NAVI run cancelled: "+reason)
				} else {
					b.sendMessage(ctx, "NAVI run cancelled: "+reason, nil)
				}
			case "run.completed":
				stream, active := b.chatStreamSnapshot(naviChatID)
				finalMessageID, _ := frame.Event.Payload["final_message_id"].(string)
				slog.Info("telegram: terminal live event received", "event_type", frame.Event.Type, "seq", frame.Event.Seq, "chat_id", naviChatID, "chat_id", chatID, "placeholder_active", active, "placeholder_message_id", stream.MessageID, "final_message_id", strings.TrimSpace(finalMessageID))
				b.stopSessionThinking(naviChatID)
				if !active || chatID == 0 {
					b.clearSessionStream(naviChatID)
					continue
				}
				b.scheduleCompletionRepair(ctx, naviChatID, chatID, stream.MessageID, finalMessageID)
			}
		}
	}
}

func readLiveConnectAck(conn *websocket.Conn) (int64, error) {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	var frame struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			CursorSeq int64 `json:"cursor_seq"`
			AfterSeq  int64 `json:"after_seq"`
		} `json:"result"`
	}
	if err := json.Unmarshal(msg, &frame); err != nil {
		return 0, err
	}
	if frame.Type != "res" || !frame.OK {
		if strings.TrimSpace(frame.Error) == "" {
			frame.Error = "live connect failed"
		}
		return 0, errors.New(frame.Error)
	}
	if frame.Result.CursorSeq != 0 {
		return frame.Result.CursorSeq, nil
	}
	return frame.Result.AfterSeq, nil
}

// handleStatusCommand fetches agent and system status from the gateway and
// formats a human-readable multi-line report. No LLM calls are involved.
func (b *Bot) handleStatusCommand(ctx context.Context, sendReply func(string, any)) {
	// Fetch agent activity status and system status in parallel.
	type result struct {
		data []byte
		err  error
	}
	agentCh := make(chan result, 1)
	sysCh := make(chan result, 1)

	go func() {
		d, e := b.gatewayDo(ctx, "GET", "/api/agent/status", nil)
		agentCh <- result{d, e}
	}()
	go func() {
		d, e := b.gatewayDo(ctx, "GET", "/api/status", nil)
		sysCh <- result{d, e}
	}()

	agentRes := <-agentCh
	sysRes := <-sysCh

	var sb strings.Builder

	// Agent activity status.
	if agentRes.err != nil {
		sb.WriteString("Agent: unavailable\n")
	} else {
		var agent map[string]any
		if err := json.Unmarshal(agentRes.data, &agent); err == nil {
			state, _ := agent["state"].(string)
			sb.WriteString(fmt.Sprintf("Agent: %s", strings.ToUpper(state)))
			if detail, _ := agent["current_detail"].(string); detail != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", detail))
			}
			sb.WriteString("\n")
			if turns, ok := agent["turns_processed"].(float64); ok {
				sb.WriteString(fmt.Sprintf("Turns processed: %d\n", int64(turns)))
			}
		}
	}

	// System status.
	if sysRes.err == nil {
		var sys map[string]any
		if err := json.Unmarshal(sysRes.data, &sys); err == nil {
			// Governor.
			if gov, ok := sys["governor"].(map[string]any); ok {
				tripped, _ := gov["tripped"].(bool)
				if tripped {
					sb.WriteString("Governor: TRIPPED\n")
				} else {
					budgetUsed, _ := gov["budget_used"].(float64)
					budgetMax, _ := gov["budget_max"].(float64)
					sb.WriteString(fmt.Sprintf("Budget: %d/%d actions\n", int(budgetUsed), int(budgetMax)))
				}
			}
			if proposals, ok := sys["proposals"].(map[string]any); ok {
				if pendingCount, ok := proposals["pending_count"].(float64); ok && int(pendingCount) > 0 {
					sb.WriteString(fmt.Sprintf("Pending proposals: %d\n", int(pendingCount)))
				}
			}
			// Connectors.
			if conn, ok := sys["connectors"].(map[string]any); ok {
				running, _ := conn["running"].(float64)
				total, _ := conn["total"].(float64)
				sb.WriteString(fmt.Sprintf("Connectors: %d/%d running\n", int(running), int(total)))
			}
			// LLM.
			if l, ok := sys["llm"].(map[string]any); ok {
				if configured, _ := l["configured"].(bool); configured {
					status, _ := l["status"].(string)
					sb.WriteString(fmt.Sprintf("LLM: %s\n", status))
				} else {
					sb.WriteString("LLM: not configured\n")
				}
			}
			// Degraded.
			if degraded, _ := sys["degraded"].(bool); degraded {
				sb.WriteString("System: DEGRADED\n")
			}
		}
	}

	msg := strings.TrimSpace(sb.String())
	if msg == "" {
		msg = "Status unavailable."
	}
	sendReply(msg, nil)
}

func telegramMessageRef(msg *tgMessage) string {
	if msg == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d", msg.Chat.ID, msg.MessageThreadID, msg.MessageID)
}

func telegramMessageIdempotencyKey(msg *tgMessage) string {
	if msg == nil {
		return ""
	}
	return "telegram:" + telegramMessageRef(msg)
}
