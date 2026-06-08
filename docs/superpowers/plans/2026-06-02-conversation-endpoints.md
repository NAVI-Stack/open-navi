# Durable Conversation Endpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build first-class durable conversation endpoints so NAVI can distinguish console visibility from connector delivery, bind Telegram chats durably, and send to attached endpoints without accidental broadcast.

**Architecture:** Add endpoint and delivery policy state to the NAVI chat store, then route message origin and assistant delivery through endpoint-aware services. Telegram becomes the first connector consumer of the endpoint resolver; Discord remains readiness-only through the shared endpoint contract.

**Tech Stack:** Go 1.24, raw SQLite via `database/sql`, existing NAVI runtime/store/gateway packages, existing connector manager retry policy, React/TypeScript console surface.

---

## Scope Boundaries

This is a forward-only tranche. Do not implement historical backfill, existing database compatibility migration beyond normal fresh-schema declarations, Discord connector behavior, multi-user permissions, participant graphs, multimodal delivery, or connector discovery.

Default delivery is `reply_to_origin`: assistant replies go only to the endpoint that produced the inbound user message, while console transcript visibility remains available through existing chat/live APIs.

## File Structure

- Modify: `docs/superpowers/specs/2026-06-02-conversation-endpoints-design.md`
  - Already clarified with dedupe, `explicit_only`, disabled receive, connector instance identity, and retry semantics.
- Create: `internal/navi/conversation_endpoint.go`
  - Public NAVI domain types, status/mode constants, input structs, store interfaces, and errors.
- Modify: `internal/navi/chat_message.go`
  - Add `OriginEndpointID`.
- Modify: `internal/runtime/inbox.go`
  - Add `OriginEndpointID` to `InboxItem` and `MessageInput`.
- Modify: `internal/runtime/run.go`
  - Add `OriginEndpointID` to `RunState` and checkpoints only if needed for delivery correlation.
- Modify: `internal/runtime/coordinator.go`
  - Preserve origin endpoint into runs and call an assistant-message delivery callback after assistant messages are persisted.
- Create: `internal/navi/store/conversation_endpoint_store.go`
  - Raw-SQL endpoint, delivery policy, and delivery attempt store.
- Modify: `internal/navi/store/schema.go`
  - Add endpoint/policy/delivery tables and origin endpoint columns to fresh NAVI schema.
- Modify: `internal/navi/store/chat_store.go`
  - Persist and scan `origin_endpoint_id` on chat messages.
- Modify: `internal/navi/store/runtime_store.go`
  - Persist and scan `origin_endpoint_id` on inbox items/runs; pass it to assistant messages.
- Modify: `internal/navi/store/runtime_session_store.go`
  - No behavior change expected; only adjust if origin endpoint needs session metadata.
- Create: `internal/navi/endpoint_resolver.go`
  - Resolve console and connector origins, enforce disabled receive semantics.
- Modify: `internal/navi/message_intake.go`
  - Resolve origin endpoint before appending user messages and submitting runtime inbox items.
- Modify: `internal/navi/navi.go`
  - Wire endpoint store/resolver/delivery service and expose endpoint APIs for gateway handlers.
- Modify: `internal/navi/config.go`
  - Add endpoint/delivery dependencies, including a connector dispatcher interface.
- Create: `internal/navi/delivery_service.go`
  - Resolve delivery targets, dedupe endpoints, record attempts, and dispatch connector sends.
- Modify: `connectors/connector.go`
  - Add delivery correlation fields to `OutboundMessage`.
- Modify: `internal/connectors/manager.go`
  - Add delivery observer/callback support without changing the existing retry policy.
- Modify: `internal/connectors/worker.go`
  - Notify delivery callbacks for queued, attempts, sent, failed, and skipped outcomes.
- Create: `internal/gateway/conversation_endpoints.go`
  - Endpoint listing, endpoint mutation, delivery policy, explicit send, and connector resolve handlers.
- Modify: `internal/gateway/server.go`
  - Register endpoint routes and pass connector manager/delivery dependencies.
- Modify: `cmd/navid/main.go`
  - Wire connector manager into NAVI delivery service config.
- Modify: `plugins/telegram/connectors/telegram/bot.go`
  - Resolve durable endpoints for inbound messages and remove first-public-chat recovery from session binding.
- Modify: `plugins/telegram/connectors/telegram/bot_test.go`
  - Add endpoint resolve tests and remove expectations for first-chat recovery.
- Modify: `web-src/navi-console/src/types/api.ts`
  - Add endpoint response schemas.
- Modify: `web-src/navi-console/src/api/chats.ts`
  - Add endpoint API hooks.
- Create: `web-src/navi-console/src/components/chat/EndpointBadges.tsx`
  - Render endpoint badges in chat details/header area.
- Modify: `web-src/navi-console/src/pages/ChatPage.tsx`
  - Display endpoint badges without changing chat list filtering.

---

### Task 1: Schema And Domain Types

**Files:**
- Create: `internal/navi/conversation_endpoint.go`
- Modify: `internal/navi/chat_message.go`
- Modify: `internal/runtime/inbox.go`
- Modify: `internal/runtime/run.go`
- Modify: `internal/navi/store/schema.go`
- Modify: `internal/navi/store/chat_store.go`
- Modify: `internal/navi/store/runtime_store.go`
- Test: `internal/navi/store/conversation_endpoint_schema_test.go`

- [ ] **Step 1: Write the failing schema test**

Create `internal/navi/store/conversation_endpoint_schema_test.go`:

```go
package store

import (
	"context"
	"database/sql"
	"testing"

	corestore "github.com/open-navi/navi/internal/store"
)

func TestMigrateSchemaCreatesConversationEndpointTables(t *testing.T) {
	ctx := context.Background()
	db, err := corestore.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	for _, table := range []string{
		"navi_conversation_endpoints",
		"navi_chat_delivery_policy",
		"navi_message_deliveries",
	} {
		if !testTableExists(t, db, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	for _, check := range []struct {
		table  string
		column string
	}{
		{"navi_chat_messages", "origin_endpoint_id"},
		{"navi_inbox", "origin_endpoint_id"},
		{"runtime_runs", "origin_endpoint_id"},
	} {
		if !testColumnExists(t, db, check.table, check.column) {
			t.Fatalf("expected %s.%s to exist", check.table, check.column)
		}
	}
}

func TestMessageDeliveriesUniqueByMessageAndEndpoint(t *testing.T) {
	ctx := context.Background()
	db, err := corestore.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO navi_message_deliveries (
			delivery_id, message_id, chat_id, endpoint_id, status, created_at, updated_at
		) VALUES
			('d1', 'm1', 'c1', 'e1', 'queued', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('d2', 'm1', 'c1', 'e1', 'queued', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err == nil {
		t.Fatal("expected duplicate message/endpoint delivery insert to fail")
	}
}

func testTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	return err == nil
}

func testColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info("` + table + `")`)
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the schema test and verify RED**

Run:

```bash
go test ./internal/navi/store -run 'TestMigrateSchemaCreatesConversationEndpointTables|TestMessageDeliveriesUniqueByMessageAndEndpoint' -count=1
```

Expected: FAIL because endpoint tables and origin columns do not exist.

- [ ] **Step 3: Add domain types**

Create `internal/navi/conversation_endpoint.go`:

```go
package navi

import (
	"context"
	"errors"
	"time"
)

type ConversationEndpointType string
type ConversationEndpointStatus string
type DeliveryPolicyMode string
type MessageDeliveryStatus string

const (
	EndpointTypeConsole   ConversationEndpointType = "console"
	EndpointTypeConnector ConversationEndpointType = "connector"

	EndpointStatusActive   ConversationEndpointStatus = "active"
	EndpointStatusDisabled ConversationEndpointStatus = "disabled"
	EndpointStatusDeleted  ConversationEndpointStatus = "deleted"

	DeliveryPolicyReplyToOrigin DeliveryPolicyMode = "reply_to_origin"
	DeliveryPolicyExplicitOnly  DeliveryPolicyMode = "explicit_only"

	MessageDeliveryQueued  MessageDeliveryStatus = "queued"
	MessageDeliverySent    MessageDeliveryStatus = "sent"
	MessageDeliveryFailed  MessageDeliveryStatus = "failed"
	MessageDeliverySkipped MessageDeliveryStatus = "skipped"
)

var (
	ErrConversationEndpointNotFound        = errors.New("navi: conversation endpoint not found")
	ErrConversationEndpointReceiveDisabled = errors.New("navi: conversation endpoint receive disabled")
	ErrConversationEndpointSendDisabled    = errors.New("navi: conversation endpoint send disabled")
	ErrConversationEndpointChatMismatch    = errors.New("navi: conversation endpoint does not belong to chat")
)

type ConversationEndpoint struct {
	ID                  ID                         `json:"endpoint_id"`
	ChatID              ID                         `json:"chat_id"`
	Type                ConversationEndpointType   `json:"endpoint_type"`
	ConnectorKind       string                     `json:"connector_kind"`
	ConnectorInstanceID string                     `json:"connector_instance_id"`
	ExternalChatID      string                     `json:"external_chat_id"`
	ExternalThreadID    string                     `json:"external_thread_id"`
	DisplayName         string                     `json:"display_name"`
	ReceiveEnabled      bool                       `json:"receive_enabled"`
	SendEnabled         bool                       `json:"send_enabled"`
	MirrorEnabled       bool                       `json:"mirror_enabled"`
	Status              ConversationEndpointStatus `json:"status"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
	Metadata            map[string]any             `json:"metadata,omitempty"`
}

type UpsertConnectorEndpointInput struct {
	ChatID              string
	ConnectorKind       string
	ConnectorInstanceID string
	ExternalChatID      string
	ExternalThreadID    string
	DisplayName         string
	Metadata            map[string]any
}

type DeliveryPolicy struct {
	ChatID    ID                 `json:"chat_id"`
	Mode      DeliveryPolicyMode `json:"default_mode"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
}

type MessageDelivery struct {
	ID                  string                `json:"delivery_id"`
	MessageID           string                `json:"message_id"`
	ChatID              string                `json:"chat_id"`
	EndpointID          string                `json:"endpoint_id"`
	ConnectorInstanceID string                `json:"connector_instance_id"`
	Status              MessageDeliveryStatus `json:"status"`
	AttemptCount        int                   `json:"attempt_count"`
	LastError           string                `json:"last_error,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
	Metadata            map[string]any        `json:"metadata,omitempty"`
}

type ConversationEndpointStore interface {
	EnsureConsoleEndpoint(ctx context.Context, chatID string) (*ConversationEndpoint, error)
	UpsertConnectorEndpoint(ctx context.Context, input UpsertConnectorEndpointInput) (*ConversationEndpoint, bool, error)
	ResolveConnectorEndpoint(ctx context.Context, connectorInstanceID, externalChatID, externalThreadID string) (*ConversationEndpoint, error)
	ListConversationEndpoints(ctx context.Context, chatID string) ([]ConversationEndpoint, error)
	GetConversationEndpoint(ctx context.Context, endpointID string) (*ConversationEndpoint, error)
	UpdateConversationEndpoint(ctx context.Context, endpoint ConversationEndpoint) error
	GetDeliveryPolicy(ctx context.Context, chatID string) (*DeliveryPolicy, error)
	SetDeliveryPolicy(ctx context.Context, policy DeliveryPolicy) error
	CreateMessageDelivery(ctx context.Context, delivery MessageDelivery) (*MessageDelivery, error)
	RecordMessageDeliveryAttempt(ctx context.Context, deliveryID string, status MessageDeliveryStatus, lastError string) error
}
```

- [ ] **Step 4: Add origin fields**

Modify:

```go
// internal/navi/chat_message.go
OriginEndpointID string `json:"originEndpointId,omitempty"`

// internal/runtime/inbox.go
OriginEndpointID string `json:"origin_endpoint_id,omitempty"`

// internal/runtime/run.go
OriginEndpointID string `json:"origin_endpoint_id,omitempty"`
```

- [ ] **Step 5: Add fresh schema declarations**

Modify `internal/navi/store/schema.go` by adding endpoint tables to `createNaviTables`, adding `origin_endpoint_id` to fresh `navi_chat_messages`, `navi_inbox`, and `runtime_runs`, and creating:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_endpoint_console_active
ON navi_conversation_endpoints(chat_id)
WHERE endpoint_type = 'console' AND status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_endpoint_external_active
ON navi_conversation_endpoints(connector_instance_id, external_chat_id, external_thread_id)
WHERE endpoint_type = 'connector' AND status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS idx_message_deliveries_message_endpoint
ON navi_message_deliveries(message_id, endpoint_id);
```

Do not add historical backfill loops for existing chats.

- [ ] **Step 6: Persist and scan origin fields**

Update insert/select/scan paths in `chat_store.go` and `runtime_store.go` so `origin_endpoint_id` round-trips for chat messages, inbox items, and runtime runs.

- [ ] **Step 7: Run schema tests and verify GREEN**

Run:

```bash
go test ./internal/navi/store -run 'ConversationEndpoint|MessageDeliveries|ChatMessage|Runtime' -count=1
```

Expected: PASS for new schema tests; unrelated tests in the package should remain green.

---

### Task 2: Endpoint Store

**Files:**
- Create: `internal/navi/store/conversation_endpoint_store.go`
- Test: `internal/navi/store/conversation_endpoint_store_test.go`

- [ ] **Step 1: Write failing store tests**

Create tests covering:

```go
func TestConversationEndpointStoreEnsuresConsoleEndpoint(t *testing.T) {}
func TestConversationEndpointStoreUpsertsConnectorEndpointByExternalAddress(t *testing.T) {}
func TestConversationEndpointStoreRejectsReceiveDisabledResolverFallback(t *testing.T) {}
func TestConversationEndpointStoreDeliveryPolicyDefaultsReplyToOrigin(t *testing.T) {}
func TestConversationEndpointStoreRecordsDeliveryAttempts(t *testing.T) {}
```

The disabled receive test should:

1. Create chat `chat-1`.
2. Upsert Telegram endpoint `(telegram, 123, "")`.
3. Set `ReceiveEnabled = false`.
4. Resolve the same external address.
5. Assert the endpoint is returned with disabled state, not silently replaced.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/navi/store -run ConversationEndpointStore -count=1
```

Expected: FAIL because store methods do not exist.

- [ ] **Step 3: Implement store methods with raw SQL**

Implement in `conversation_endpoint_store.go`:

```go
func (s *SQLiteStore) EnsureConsoleEndpoint(ctx context.Context, chatID string) (*navi.ConversationEndpoint, error)
func (s *SQLiteStore) UpsertConnectorEndpoint(ctx context.Context, input navi.UpsertConnectorEndpointInput) (*navi.ConversationEndpoint, bool, error)
func (s *SQLiteStore) ResolveConnectorEndpoint(ctx context.Context, connectorInstanceID, externalChatID, externalThreadID string) (*navi.ConversationEndpoint, error)
func (s *SQLiteStore) ListConversationEndpoints(ctx context.Context, chatID string) ([]navi.ConversationEndpoint, error)
func (s *SQLiteStore) GetConversationEndpoint(ctx context.Context, endpointID string) (*navi.ConversationEndpoint, error)
func (s *SQLiteStore) UpdateConversationEndpoint(ctx context.Context, endpoint navi.ConversationEndpoint) error
func (s *SQLiteStore) GetDeliveryPolicy(ctx context.Context, chatID string) (*navi.DeliveryPolicy, error)
func (s *SQLiteStore) SetDeliveryPolicy(ctx context.Context, policy navi.DeliveryPolicy) error
func (s *SQLiteStore) CreateMessageDelivery(ctx context.Context, delivery navi.MessageDelivery) (*navi.MessageDelivery, error)
func (s *SQLiteStore) RecordMessageDeliveryAttempt(ctx context.Context, deliveryID string, status navi.MessageDeliveryStatus, lastError string) error
```

Use `uuid.NewString()` for missing IDs, `time.Now().UTC()` for timestamps, and JSON helpers matching nearby store files.

- [ ] **Step 4: Run store tests and verify GREEN**

Run:

```bash
go test ./internal/navi/store -run ConversationEndpointStore -count=1
```

Expected: PASS.

---

### Task 3: Endpoint Resolver And Message Intake Origin

**Files:**
- Create: `internal/navi/endpoint_resolver.go`
- Modify: `internal/navi/message_intake.go`
- Modify: `internal/navi/navi.go`
- Modify: `internal/runtime/coordinator.go`
- Test: `internal/navi/endpoint_resolver_test.go`
- Test: `internal/navi/message_intake_endpoint_test.go`
- Test: `internal/runtime/coordinator_test.go`

- [ ] **Step 1: Write failing resolver tests**

Create tests proving:

```go
func TestEndpointResolverCreatesChatConsoleAndConnectorEndpointForNewInboundConnector(t *testing.T) {}
func TestEndpointResolverReturnsDisabledReceiveWithoutFallback(t *testing.T) {}
func TestMessageIntakeDefaultsConsoleOriginForConsoleMessage(t *testing.T) {}
func TestMessageIntakePersistsConnectorOriginEndpoint(t *testing.T) {}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/navi -run 'EndpointResolver|MessageIntake.*Endpoint' -count=1
```

Expected: FAIL because resolver and intake origin support do not exist.

- [ ] **Step 3: Implement resolver**

Create:

```go
type EndpointResolver struct {
	Chats     ChatStore
	Endpoints ConversationEndpointStore
}

type ResolveConnectorEndpointRequest struct {
	ConnectorKind       string
	ConnectorInstanceID string
	ExternalChatID      string
	ExternalThreadID    string
	DisplayName         string
	ExperienceMode      string
}

type ResolveEndpointResult struct {
	ChatID          string
	EndpointID      string
	CreatedChat     bool
	CreatedEndpoint bool
	Endpoint         ConversationEndpoint
}
```

The resolver must return `ErrConversationEndpointReceiveDisabled` when the external address resolves to a disabled or non-active endpoint.

- [ ] **Step 4: Wire intake origin**

Extend `SubmitMessageRequest` and `SubmitMessageResult`:

```go
OriginEndpointID string
```

In `MessageIntakeService.SubmitMessage`:

1. If `OriginEndpointID` is blank, resolve or create console endpoint for the chat.
2. Store `OriginEndpointID` on the user `ChatMessage`.
3. Pass `OriginEndpointID` into `naviruntime.MessageInput`.

- [ ] **Step 5: Preserve origin into runtime runs**

In `internal/runtime/coordinator.go`, copy `InboxItem.OriginEndpointID` into:

```go
run.OriginEndpointID = strings.TrimSpace(item.OriginEndpointID)
run.SetScratchpadValue("origin_endpoint_id", run.OriginEndpointID)
```

Do this when launching a run from an inbox item.

- [ ] **Step 6: Run resolver/intake tests and verify GREEN**

Run:

```bash
go test ./internal/navi ./internal/runtime -run 'EndpointResolver|MessageIntake.*Endpoint|OriginEndpoint' -count=1
```

Expected: PASS.

---

### Task 4: Delivery Service And Connector Retry Observability

**Files:**
- Create: `internal/navi/delivery_service.go`
- Modify: `internal/navi/config.go`
- Modify: `internal/navi/navi.go`
- Modify: `connectors/connector.go`
- Modify: `internal/connectors/manager.go`
- Modify: `internal/connectors/worker.go`
- Modify: `internal/runtime/coordinator.go`
- Test: `internal/navi/delivery_service_test.go`
- Test: `internal/connectors/worker_test.go`
- Test: `internal/runtime/coordinator_test.go`

- [ ] **Step 1: Write failing delivery service tests**

Create tests:

```go
func TestDeliveryServiceReplyToOriginDispatchesOnlyOriginConnector(t *testing.T) {}
func TestDeliveryServiceExplicitOnlySkipsConnectorDispatchWithoutExplicitTargets(t *testing.T) {}
func TestDeliveryServiceDedupesOriginMirrorAndExplicitTargets(t *testing.T) {}
func TestDeliveryServiceRejectsEndpointOutsideChat(t *testing.T) {}
func TestDeliveryServiceRecordsSkippedDisabledEndpoint(t *testing.T) {}
```

The dedupe test must construct a target list where the same endpoint appears as origin, mirror-enabled, and explicit; assert one delivery record and one dispatch.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/navi -run DeliveryService -count=1
```

Expected: FAIL because the service does not exist.

- [ ] **Step 3: Add connector delivery correlation**

Modify `connectors.OutboundMessage`:

```go
DeliveryID string
EndpointID string
```

Add a manager callback type:

```go
type DeliveryObserver func(ctx context.Context, deliveryID string, status string, err error)
```

Add it to `internal/connectors.ManagerConfig` and call it from `worker.go`:

- before first attempt: `queued`
- before/after each retry attempt: increment attempt via callback
- on success: `sent`
- on exhausted error: `failed`
- on hook cancellation: `skipped`

Do not change retry counts or retry timing; keep the existing worker retry policy authoritative.

- [ ] **Step 4: Implement DeliveryService**

Implement:

```go
type ConnectorDispatcher interface {
	Dispatch(ctx context.Context, connectorName string, msg connectors.OutboundMessage) error
}

type DeliveryService struct {
	Endpoints  ConversationEndpointStore
	Dispatcher ConnectorDispatcher
}

type DeliverAssistantMessageInput struct {
	ChatID             string
	MessageID          string
	Content            string
	OriginEndpointID   string
	RuntimeSessionID   string
	RunID              string
	ExplicitEndpointIDs []string
}
```

Rules:

- Read policy with default `reply_to_origin`.
- If policy is `explicit_only` and no explicit endpoint IDs are present, return without connector dispatch.
- Combine origin connector endpoint, mirror-enabled endpoints, and explicit endpoint IDs.
- Dedupe by endpoint ID before creating `navi_message_deliveries`.
- Create one delivery row per target before dispatch.
- Dispatch through the connector manager with `DeliveryID` and `EndpointID`.
- Never call `DispatchAll`.

- [ ] **Step 5: Wire runtime completion callback**

Add a callback to `RunCoordinator`:

```go
type AssistantMessageDeliverer func(ctx context.Context, run *RunState, messageID string, content string, inboxItemID string) error
```

Call it after successful `CompleteRun` and scheduled `AppendAssistantMessage` calls. Configure it in `navi.New` to call `DeliveryService.DeliverAssistantMessage`.

- [ ] **Step 6: Run delivery tests and verify GREEN**

Run:

```bash
go test ./internal/navi ./internal/connectors ./internal/runtime -run 'DeliveryService|DeliveryObserver|AssistantMessageDeliverer' -count=1
```

Expected: PASS.

---

### Task 5: Gateway Endpoint APIs

**Files:**
- Create: `internal/gateway/conversation_endpoints.go`
- Modify: `internal/gateway/server.go`
- Test: `internal/gateway/conversation_endpoints_test.go`

- [ ] **Step 1: Write failing gateway tests**

Create tests for:

```go
func TestGatewayListsConversationEndpoints(t *testing.T) {}
func TestGatewayAttachesConnectorEndpoint(t *testing.T) {}
func TestGatewayUpdatesDeliveryPolicy(t *testing.T) {}
func TestGatewayExplicitSendRejectsEndpointOutsideChat(t *testing.T) {}
func TestGatewayConnectorResolveRejectsDisabledReceiveEndpoint(t *testing.T) {}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/gateway -run 'ConversationEndpoint|ExplicitSend|ConnectorResolve' -count=1
```

Expected: FAIL because routes and handlers do not exist.

- [ ] **Step 3: Register routes**

In `server.go` register:

```go
s.mux.Handle("GET /api/navi/chats/{id}/endpoints", protect(s.handleListConversationEndpoints))
s.mux.Handle("POST /api/navi/chats/{id}/endpoints", protect(s.handleAttachConversationEndpoint))
s.mux.Handle("PATCH /api/navi/chats/{id}/endpoints/{endpoint_id}", protect(s.handleUpdateConversationEndpoint))
s.mux.Handle("GET /api/navi/chats/{id}/delivery-policy", protect(s.handleGetChatDeliveryPolicy))
s.mux.Handle("PATCH /api/navi/chats/{id}/delivery-policy", protect(s.handleSetChatDeliveryPolicy))
s.mux.Handle("POST /api/navi/chats/{id}/send", protect(s.handleExplicitEndpointSend))
s.mux.Handle("POST /api/connectors/endpoints/resolve", protect(s.handleResolveConnectorEndpoint))
```

- [ ] **Step 4: Implement handlers**

Handlers should use `s.cfg.Navi` methods rather than raw SQL in gateway. Add NAVI facade methods:

```go
func (n *NAVI) ListConversationEndpoints(ctx context.Context, chatID string) ([]ConversationEndpoint, error)
func (n *NAVI) AttachConversationEndpoint(ctx context.Context, chatID string, input UpsertConnectorEndpointInput) (*ConversationEndpoint, error)
func (n *NAVI) SetConversationEndpoint(ctx context.Context, endpoint ConversationEndpoint) error
func (n *NAVI) GetChatDeliveryPolicy(ctx context.Context, chatID string) (*DeliveryPolicy, error)
func (n *NAVI) SetChatDeliveryPolicy(ctx context.Context, policy DeliveryPolicy) error
func (n *NAVI) SendToConversationEndpoints(ctx context.Context, chatID, content string, endpointIDs []string) error
func (n *NAVI) ResolveConnectorConversationEndpoint(ctx context.Context, req ResolveConnectorEndpointRequest) (*ResolveEndpointResult, error)
```

- [ ] **Step 5: Run gateway tests and verify GREEN**

Run:

```bash
go test ./internal/gateway -run 'ConversationEndpoint|ExplicitSend|ConnectorResolve' -count=1
```

Expected: PASS.

---

### Task 6: Telegram Durable Endpoint Binding

**Files:**
- Modify: `plugins/telegram/connectors/telegram/bot.go`
- Modify: `plugins/telegram/connectors/telegram/bot_test.go`

- [ ] **Step 1: Write failing Telegram tests**

Add tests:

```go
func TestTelegramEnsureChatUsesConnectorEndpointResolve(t *testing.T) {}
func TestTelegramDisabledEndpointDoesNotRecoverToFirstPublicChat(t *testing.T) {}
func TestTelegramQueuesOriginEndpointID(t *testing.T) {}
```

The disabled endpoint test should configure the fake gateway to return a disabled receive response and assert the bot does not call `GET /api/navi/chats` as fallback.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./plugins/telegram/connectors/telegram -run 'Endpoint|Recover|OriginEndpoint' -count=1
```

Expected: FAIL because Telegram still uses old recovery.

- [ ] **Step 3: Add endpoint resolve client**

Add a Telegram helper:

```go
type resolvedEndpoint struct {
	ChatID          string `json:"chat_id"`
	EndpointID      string `json:"endpoint_id"`
	CreatedChat     bool   `json:"created_chat"`
	CreatedEndpoint bool   `json:"created_endpoint"`
}

func (b *Bot) resolveConversationEndpoint(ctx context.Context, chatID int64, threadID int64, displayName string) (*resolvedEndpoint, error)
```

It should call `POST /api/connectors/endpoints/resolve` with:

```json
{
  "connector_kind": "telegram",
  "connector_instance_id": "<b.Name()>",
  "external_chat_id": "<telegram chat id>",
  "external_thread_id": "<thread id or empty>",
  "display_name": "<best label>"
}
```

- [ ] **Step 4: Replace first-chat recovery**

Modify `ensureChat` / `recoverSessionForChat` flow:

- Resolve by endpoint first.
- If disabled receive is returned, do not create a replacement chat or endpoint.
- Bind in-memory maps only after durable endpoint resolution succeeds.
- Store endpoint ID in the queue request.

- [ ] **Step 5: Queue origin endpoint ID**

Modify `queueSessionMessageOnce` body to include:

```json
"origin_endpoint_id": endpointID
```

and update gateway message handler/request shape to accept it.

- [ ] **Step 6: Run Telegram tests and verify GREEN**

Run:

```bash
go test ./plugins/telegram/connectors/telegram -run 'Endpoint|Recover|OriginEndpoint' -count=1
```

Expected: PASS.

---

### Task 7: Telegram-Origin Assistant Delivery Through Delivery Service

**Files:**
- Modify: `plugins/telegram/connectors/telegram/bot.go`
- Modify: `plugins/telegram/connectors/telegram/bot_test.go`
- Test: `internal/navi/delivery_service_test.go`

- [ ] **Step 1: Write failing no-duplicate-delivery tests**

Add tests proving a Telegram-origin run does not both:

- send through old `/ws/live` finalization, and
- send through the new delivery service.

Test name:

```go
func TestTelegramOriginAssistantReplyDeliveredOnceThroughDeliveryService(t *testing.T) {}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./plugins/telegram/connectors/telegram ./internal/navi -run 'DeliveredOnce|TelegramOrigin' -count=1
```

Expected: FAIL until old assistant reply finalization is removed or guarded.

- [ ] **Step 3: Guard old live delivery path**

Modify `sessionLiveLoop` so ordinary `assistant.message.completed` final replies are not sent directly for endpoint-bound chats. Keep operational events such as proposal waiting and run failure/cancellation notices only where they are not assistant-message delivery.

If the implementation keeps partial streaming temporarily, final delivery must still be owned by Delivery Service and must not duplicate endpoint delivery records.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
go test ./plugins/telegram/connectors/telegram ./internal/navi -run 'DeliveredOnce|TelegramOrigin' -count=1
```

Expected: PASS.

---

### Task 8: Console Endpoint Visibility

**Files:**
- Modify: `web-src/navi-console/src/types/api.ts`
- Modify: `web-src/navi-console/src/api/chats.ts`
- Create: `web-src/navi-console/src/components/chat/EndpointBadges.tsx`
- Modify: `web-src/navi-console/src/pages/ChatPage.tsx`
- Test: `web-src/navi-console/src/components/chat/EndpointBadges.test.tsx`

- [ ] **Step 1: Write failing frontend tests**

Add tests that render:

- console-only endpoint badge
- Telegram endpoint badge
- mirrored endpoint badge

Expected test names:

```ts
it('renders attached endpoint badges for the active chat', async () => {})
it('does not change chat list filtering when endpoints are present', async () => {})
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
npm.cmd run test:run -- --runInBand
```

Expected: FAIL for missing endpoint badge component/API.

- [ ] **Step 3: Add schemas and API hook**

Add endpoint schema to `types/api.ts` and a hook:

```ts
export function useChatEndpoints(id: string | null) {
  const url = id ? `/api/navi/chats/${id}/endpoints` : null;
  return useQuery({
    queryKey: ['chat-endpoints', id],
    queryFn: () => naviFetch(url!, ChatEndpointsSchema),
    enabled: !!id,
  });
}
```

- [ ] **Step 4: Render badges without changing chat list model**

Use the active chat endpoint hook only in the active chat surface. Do not modify `hasChatMessages` or recents filtering.

- [ ] **Step 5: Run frontend tests and verify GREEN**

Run:

```bash
npm.cmd run test:run -- --runInBand
```

Expected: PASS.

---

### Task 9: Endpoint-Aware Tools After Core Tests Pass

**Files:**
- Modify: `internal/navi/runtime_executor.go`
- Modify: `internal/navi/tool_registry_test.go`
- Modify: `internal/navi/tool_runtime_helpers.go`
- Test: `internal/navi/tool_runtime_helpers_test.go`

- [ ] **Step 1: Verify prerequisite test gates**

Run:

```bash
go test ./internal/navi/store ./internal/navi ./internal/gateway ./plugins/telegram/connectors/telegram -count=1
```

Expected: PASS before adding endpoint-aware tools.

- [ ] **Step 2: Write failing tool tests**

Add tests:

```go
func TestListEndpointsToolReturnsAttachedSendEnabledEndpoints(t *testing.T) {}
func TestSendToEndpointToolRejectsUnknownEndpoint(t *testing.T) {}
func TestSendToEndpointToolDispatchesOnlyNamedEndpoints(t *testing.T) {}
```

- [ ] **Step 3: Run tests and verify RED**

Run:

```bash
go test ./internal/navi -run 'EndpointTool|SendToEndpoint|ListEndpoints' -count=1
```

Expected: FAIL because tools are not registered.

- [ ] **Step 4: Register governed endpoint tools**

Add:

- `navi.messaging.list_endpoints`
- `navi.messaging.send_to_endpoint`

Do not change `navi.messaging.send_reply`; it remains current-chat only.

Tool execution must use durable endpoint IDs returned by `list_endpoints`. It must not accept raw connector instance IDs or external chat IDs from the model.

- [ ] **Step 5: Run tool tests and verify GREEN**

Run:

```bash
go test ./internal/navi -run 'EndpointTool|SendToEndpoint|ListEndpoints|ToolRegistry' -count=1
```

Expected: PASS.

---

### Task 10: Full Verification

**Files:**
- No new files unless failures reveal scoped fixes.

- [ ] **Step 1: Run targeted Go verification**

Run:

```bash
go test ./internal/navi/store ./internal/navi ./internal/runtime ./internal/connectors ./internal/gateway ./plugins/telegram/connectors/telegram -count=1
```

Expected: PASS.

- [ ] **Step 2: Run broader backend verification**

Run:

```bash
go test ./cmd/... ./internal/... ./connectors/... ./plugins/... -count=1
```

Expected: PASS. If unrelated pre-existing failures appear, capture exact package/test names and do not paper over them.

- [ ] **Step 3: Run frontend verification**

Run:

```bash
npm.cmd run test:run
npm.cmd run build
```

Expected: PASS. If the repo has a frontend package-specific command requirement, use the existing package script from `web-src/navi-console/package.json`.

- [ ] **Step 4: Final diff audit**

Run:

```bash
git diff --stat
git diff -- docs/superpowers/specs/2026-06-02-conversation-endpoints-design.md docs/superpowers/plans/2026-06-02-conversation-endpoints.md
```

Expected: only scoped endpoint implementation files plus the spec/plan changes. Pre-existing dirty files not touched by this work should remain unrelated.

---

## Plan Self-Review Notes

- Coverage: tasks cover schema, endpoint store, resolver, delivery service, gateway APIs, Telegram binding, Telegram-origin delivery, console visibility, and endpoint-aware tools after core tests pass.
- Scope: Discord remains readiness-only through endpoint fields. No historical backfill, participant graph, multimodal delivery, or connector discovery is included.
- Dedupe: both schema uniqueness and delivery target dedupe are explicitly tested.
- Retry: delivery status is recorded through existing connector worker retry flow; no separate retry system is introduced.
