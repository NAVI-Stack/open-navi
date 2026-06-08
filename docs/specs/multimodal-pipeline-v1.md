**Status:** Draft 1
**Last Updated:** 2026-05-25
**Updated By:** Multimodal pipeline design pass
**Scope:** Design only — no implementation in this document.

# Multimodal Message Pipeline V1

## Purpose

Today NAVI's inbound message pipeline is **text-only end to end**. This document
specifies the platform work required to carry **images and files** from any
ingress surface (gateway HTTP, connectors) through intake, the inbox, runtime
prompt assembly, and into a provider's wire format — with an honest
capability/fallback story for providers that cannot see images.

This is the cross-cutting platform initiative that blocks **T1-5 (complete media
ingestion)** in [telegram-upgrades.md](telegram-upgrades.md). T1-5 is explicitly
*deferred until this lands*; the Telegram connector currently ships only a
media-*presence* stopgap (caption/filename folded into the text envelope). Once
this pipeline exists, the connector becomes a thin producer of attachments rather
than the owner of media logic.

### Non-goals (V1)

- **Speech-to-text / voice ingestion.** Voice notes remain dropped or surfaced as
  presence-only. STT is a separate initiative (see [Future work](#9-future-work)).
- **Outbound (assistant→user) media generation.** This doc covers *ingestion*
  into the prompt only.
- **Document parsing/OCR** (PDF text extraction, etc.). V1 carries images to
  vision models and files as referenced metadata; semantic extraction is later.
- **Video/audio frames.** Out of scope for V1.

---

## 1. Current State (what exists, what's missing)

| Layer | File | State |
|-------|------|-------|
| Gateway ingress | `internal/gateway/server.go` `handleNaviSendMessage` (~2536) | Decodes only `content` / `source_channel` / `source_message_ref` / `idempotency_key`. |
| Intake DTO | `internal/runtime/inbox.go:32` `MessageInput` | Text-only (`Content string`). |
| Inbox item | `internal/runtime/inbox.go:10` `InboxItem` | `Content string`; `PayloadType` exists but is always `"text"`. |
| LLM message | `internal/llm/types.go:8` `llm.Message` | `Content string` — no typed content blocks. |
| Provider encoding | `plugins/llm-anthropic/.../convert.go`, `plugins/llm-openai/.../convert.go` | **Brittle string hack**: `buildUserContentBlocks` / `buildImageContent` sniff the `Content` *string* for `data:...;base64,` URIs or an inline OpenAI-style JSON array, then re-emit typed blocks. Ollama adapter does none of this. |
| Blob storage | `internal/blob/blob.go` | `blob.Store` (filesystem impl) with `Put/Get/Delete/Exists`. Functional but unused for chat media. |
| Linked-file model | `internal/navi/chat.go:174` `LinkedFile` | Has `StorageKey` / `MimeType` / `SizeBytes`. Not wired to inbound prompt assembly. |

### Key observation: the string hack must be retired

Both vision adapters already reconstruct typed image blocks **by parsing a magic
string out of `Content`**. This is the "data smuggled through a string" smell:

- It forces every producer to base64-encode bytes inline and shove them into a
  text field, defeating the blob store that already exists.
- It is undiscoverable (a `Content` that starts with `[` is silently
  reinterpreted as JSON), lossy, and impossible to size-budget.
- Ollama silently ignores it, so behavior diverges per provider with no signal.

V1 **replaces** this with first-class typed content blocks on `llm.Message`. The
string-sniffing paths are deleted once the typed path is wired (see
[Phase 4](#8-phased-rollout)).

---

## 2. Architecture Overview

```
                bytes                       reference (storage_key)
ingress ──────────────────────► blob.Store ◄──────────────┐
(gateway / connector)                                      │
   │ attachments[] (metadata + bytes or URL)               │
   ▼                                                        │
MessageInput.Attachments[] ──► InboxItem.Attachments[] ──► prompt assembly
   │                                                        │ fetches bytes by key,
   │                                                        │ checks provider capability
   ▼                                                        ▼
runtime intake                                    llm.Message.Parts[] (typed blocks)
                                                            │
                                                            ▼
                                                  provider adapter encodes
                                                  (Anthropic/OpenAI native;
                                                   non-vision → text fallback)
```

Two distinct planes carry an attachment:

1. **The bytes** are written to `blob.Store` as early as possible (at ingress) and
   thereafter referenced by an opaque `storage_key`. Bytes never travel inside
   `Content` or `InboxItem` JSON.
2. **The metadata** (`storage_key`, `mime_type`, `size_bytes`, `filename`, `kind`)
   travels through `MessageInput → InboxItem → ChatMessage` as structured fields.

Prompt assembly is the single place that resolves a `storage_key` back into bytes
and decides — based on **provider capability** and **mime type** — whether to emit
an image block or a text-description fallback.

---

## 3. Typed Content Blocks on `llm.Message`

### 3.1 New types (`internal/llm/types.go`)

Add a typed-parts representation while keeping `Content string` for the common
text-only path (backward compatible; the vast majority of messages stay scalar).

```go
// ContentPart is one typed segment of a multimodal message.
// Exactly one of the value fields is set, keyed by Type.
type ContentPart struct {
    Type  ContentPartType `json:"type"`            // "text" | "image"
    Text  string          `json:"text,omitempty"`  // Type == text
    Image *ImagePart      `json:"image,omitempty"` // Type == image
}

type ContentPartType string

const (
    ContentText  ContentPartType = "text"
    ContentImage ContentPartType = "image"
)

// ImagePart carries image bytes for a vision-capable provider.
// Source is resolved by prompt assembly: either inline base64 (Data) or a
// caller-supplied remote URL (URL). Adapters consume whichever is set.
type ImagePart struct {
    MediaType string `json:"media_type"`     // e.g. "image/png", "image/jpeg"
    Data      string `json:"data,omitempty"` // base64-encoded bytes (preferred)
    URL       string `json:"url,omitempty"`  // remote URL passthrough (optional)
}

// Message gains an optional Parts field. When Parts is non-empty it is the
// authoritative content and Content is ignored by adapters. When Parts is empty,
// Content (the scalar string) is used exactly as today.
type Message struct {
    Role       string        `json:"role"`
    Content    string        `json:"content,omitempty"`
    Parts      []ContentPart `json:"parts,omitempty"` // NEW — multimodal content
    ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
    ToolCallID string        `json:"tool_call_id,omitempty"`
}
```

**Invariants**

- `Parts` is only ever populated for `role == "user"` in V1. (Assistant/tool
  multimodal output is out of scope.)
- When `Parts` is set, adapters **must not** also read `Content`.
- A helper `Message.TextOnly() bool` reports whether `Parts` contains any
  non-text part — used by capability gating.
- Supported media types in V1: `image/png`, `image/jpeg`, `image/gif`,
  `image/webp` (the Anthropic/OpenAI vision intersection). Unknown types are
  treated as non-vision and fall back to text (see §4).

### 3.2 Provider capability declaration

`Provider` cannot answer "do you see images?" today. Add a capability so prompt
assembly can decide per-model rather than hard-coding provider names.

```go
// VisionCapability is implemented by providers that can accept image content
// blocks for at least some models. Discovered via type assertion (consistent
// with connectors/capabilities.go).
type VisionCapability interface {
    SupportsVision(model string) bool
}
```

- Anthropic adapter: `SupportsVision` → true for Claude 3+ / 4.x model IDs.
- OpenAI adapter: true for `gpt-4o*`, `gpt-4.1*`, etc.; false for non-vision and
  for reasoning models that reject image input.
- Ollama adapter: true only for known vision tags (e.g. `llava`, `llama3.2-vision`);
  default false.
- A provider that does **not** implement `VisionCapability` is treated as
  text-only (safe default). The `FallbackChain` reports the capability of its
  *currently selected* candidate.

### 3.3 Per-adapter encoding

Each adapter gains a typed path that consumes `Message.Parts` directly,
**replacing** the string-sniffing functions.

**Anthropic** (`plugins/llm-anthropic/.../convert.go`)
- `text` part → `antContentBlock{Type: "text", Text: ...}`.
- `image` part with `Data` → `antContentBlock{Type:"image", Source:&antImageSource{Type:"base64", MediaType:..., Data:...}}`.
- `image` part with `URL` → Anthropic `url` image source.
- `buildUserContentBlocks` / `parseDataURI` string parsing is **deleted**.

**OpenAI / OpenRouter** (`plugins/llm-openai/.../convert.go`)
- `text` part → `oaiContentBlock{Type:"text", Text:...}`.
- `image` part → `oaiContentBlock{Type:"image_url", ImageURL:{URL: dataURI-or-URL}}`,
  where a `Data` part is reassembled into a `data:<media>;base64,<data>` URI for the
  wire (OpenAI's format requires it), but bytes never originated from `Content`.
- `buildImageContent` string parsing is **deleted**.

**Ollama** (`plugins/llm-ollama/.../*`)
- For vision-capable tags: encode images per Ollama's `images: [base64,...]` field.
- Otherwise: never receives `Parts` (prompt assembly applied text fallback first).

---

## 4. Capability & Fallback Story

Prompt assembly resolves attachments into `Parts` **only after** consulting the
selected provider/model's `VisionCapability`. The decision table:

| Provider sees images? | Mime is supported image? | Result |
|---|---|---|
| yes | yes | Emit `ImagePart` (bytes from blob). |
| yes | no (e.g. PDF, zip) | Emit a **text placeholder** part describing the file (see below). |
| no  | any | Emit a **text placeholder** part; no image bytes sent. |

**Text placeholder format** (the honest-degradation envelope, consistent with
NAVI's "not-yet-executable" philosophy and the existing Telegram presence stopgap):

```
[Attachment: photo.png — image/png, 248 KB. (This model cannot view images; describe what you need from it.)]
```

- For non-vision providers, the placeholder *omits* the parenthetical capability
  note when the file is non-image (it's just a reference), but *includes* it for
  images so the model knows it is blind to content it would otherwise expect.
- The placeholder is appended to the user's text so the turn is never empty.
- The original attachment metadata is preserved on the `ChatMessage` regardless of
  fallback, so a later turn on a vision-capable model can re-attach it.

**Fallback chain interaction:** if the primary provider is non-vision but a
fallback is vision-capable, V1 does **not** reorder the chain to "find a vision
model" — that would violate deterministic routing. Capability is evaluated against
the candidate actually selected. (Vision-aware routing is [future work](#9-future-work).)

---

## 5. Attachment Plumbing (ingress → runtime)

### 5.1 Attachment metadata type

A single struct travels the pipeline. Define it in `internal/runtime` (the
narrowest shared package) and reference it from gateway and intake.

```go
// Attachment is inbound media metadata. Bytes live in blob.Store under StorageKey.
type Attachment struct {
    StorageKey string `json:"storage_key"`        // blob.Store key (authoritative)
    Kind       string `json:"kind"`               // "image" | "file"
    MimeType   string `json:"mime_type"`
    SizeBytes  int64  `json:"size_bytes"`
    Filename   string `json:"filename,omitempty"`
    Caption    string `json:"caption,omitempty"`  // e.g. Telegram caption
    SourceRef  string `json:"source_ref,omitempty"` // upstream id (e.g. telegram file_id)
}
```

### 5.2 Gateway request shape

`handleNaviSendMessage` is extended to accept attachments. Two ingress styles:

1. **`multipart/form-data`** (browser/console upload): a JSON `payload` part plus
   N binary file parts. The handler streams each file part directly into
   `blob.Store.Put` (never buffering the whole body in `Content`), derives
   `mime_type`/`size_bytes`, and builds `Attachment{StorageKey:...}`.
2. **`application/json`** (programmatic / already-stored): the body carries an
   `attachments` array referencing **pre-existing** `storage_key`s (e.g. a
   connector that already downloaded and stored bytes). No bytes inline.

```jsonc
// JSON body (extended)
{
  "content": "what's in this screenshot?",
  "source_channel": "web",
  "idempotency_key": "…",
  "attachments": [
    { "storage_key": "chat/<id>/<uuid>.png", "kind": "image",
      "mime_type": "image/png", "size_bytes": 253952, "filename": "screen.png" }
  ]
}
```

**Validation at the gateway boundary** (the only place untrusted size/type enter):
- Per-attachment max size and per-request count/total-size caps (config-driven,
  see §7). Reject with `413`/`400` before storing.
- Mime allow-list for `kind: "image"`; anything else is stored as `kind: "file"`.
- For the JSON-reference style, verify `blob.Store.Exists(storage_key)` and that
  the key is namespaced to this chat (prevents cross-chat key references).

### 5.3 Carry through MessageInput → InboxItem

```go
// internal/runtime/inbox.go
type MessageInput struct {
    Content          string       `json:"content"`
    Attachments      []Attachment `json:"attachments,omitempty"` // NEW
    // … existing fields unchanged …
}

type InboxItem struct {
    // … existing fields …
    Content     string       `json:"content,omitempty"`
    Attachments []Attachment `json:"attachments,omitempty"` // NEW
}
```

- `NewInboxItem` sets `PayloadType = "multimodal"` when `Attachments` is non-empty
  (the existing always-`"text"` field finally earns its keep).
- `InboxItem.Attachments` is persisted alongside the item (it is metadata, not
  bytes — small and safe to store inline as JSON in the inbox/queue row).
- The connectors path (`internal/connectors/*` → `queueSessionMessage`) gains the
  same `Attachments` carry. The Telegram connector's `parseMediaMetadata` +
  `getFile`/download becomes: download → `blob.Store.Put` → emit `Attachment`.

### 5.4 Persist on the chat transcript

When an inbox item is materialized into a durable `ChatMessage`, its
`Attachments` map to the existing `LinkedFile` model (`internal/navi/chat.go:174`)
— `StorageKey`/`MimeType`/`SizeBytes`/`Name` already line up. This gives the
console a render target and lets later turns re-reference prior attachments.

---

## 6. Prompt Assembly: reference → blocks

Prompt assembly (the context assembler under
`internal/navi/orchestration/context/` / message→`llm.Message` construction) is
the **only** component that turns a `storage_key` back into bytes.

For each user `ChatMessage`/`InboxItem` with attachments:

1. Start a `[]ContentPart` with the text part (the user's typed content), if any.
2. Resolve the selected model's `VisionCapability` once for the turn.
3. For each attachment, apply the §4 decision table:
   - **image block:** `blob.Store.Get(storage_key)` → read bytes → base64 →
     `ImagePart{MediaType, Data}`. Apply a **byte budget** (§7): if total image
     bytes for the turn exceed the budget, downgrade the lowest-priority images to
     text placeholders (largest-first or oldest-first; deterministic).
   - **text placeholder:** append the §4 envelope string as a `text` part.
4. If the assembled `Parts` contains only text parts, collapse back to a scalar
   `Content` string (so non-multimodal turns produce identical wire output to
   today — important for cache stability).

**Failure handling:** if `blob.Store.Get` fails (missing key, read error), assembly
emits a text placeholder noting the attachment was unavailable rather than failing
the turn. Media must never hard-fail a conversation.

**Cost/telemetry:** image parts are attributed in `CostAttribution` per the
provider's image-token accounting; the assembler tags the turn so the governor's
cost ceiling still applies.

---

## 7. Configuration

New keys under a `multimodal:` section in `config/runtime.yaml` (env overrides
follow the existing `NAVI_*` convention):

```yaml
multimodal:
  enabled: false              # master gate (Phase 1 ships dark)
  max_attachment_bytes: 5_242_880     # 5 MiB per file
  max_attachments_per_message: 8
  max_image_bytes_per_turn: 15_728_640 # 15 MiB total images sent to a model/turn
  allowed_image_types: ["image/png", "image/jpeg", "image/gif", "image/webp"]
  blob_namespace: "chat"      # blob key prefix for chat attachments
```

- `enabled: false` makes the gateway reject attachment fields and assembly ignore
  any stored attachments — the pipeline can land in `main` without behavior change.
- Budgets are enforced at the gateway (ingest) and again at assembly (send) so a
  large backlog of stored images can't blow the prompt.

---

## 8. Phased Rollout

Each phase is independently shippable and leaves `main` green. The master
`multimodal.enabled` flag stays `false` until Phase 4.

**Phase 1 — Typed content blocks (LLM core), dark.**
- Add `ContentPart`/`ImagePart`/`Message.Parts` and `VisionCapability`.
- Implement the typed encoding path in the Anthropic, OpenAI, and Ollama adapters
  *alongside* the existing string hack (string hack still active).
- Unit tests: each adapter encodes a known `Parts` payload to the correct wire
  shape; non-vision providers emit text fallback.
- No ingress changes. No behavior change for callers.

**Phase 2 — Blob + attachment plumbing.**
- Add `runtime.Attachment`, extend `MessageInput`/`InboxItem`, set
  `PayloadType="multimodal"`.
- Gateway: accept `multipart` + JSON `attachments`, store bytes to `blob.Store`,
  enforce gateway-side budgets. Still gated by `enabled` (default reject).
- Persist attachments to `LinkedFile` on the chat transcript.
- Tests: round-trip an upload → blob key → inbox item → chat message.

**Phase 3 — Prompt assembly wiring.**
- Assembly resolves `storage_key` → `Parts`, applies capability gating, byte
  budgets, and text fallback. Collapse-to-scalar for text-only turns.
- Tests: vision model gets image blocks; non-vision model gets placeholders;
  budget overflow degrades deterministically; missing blob degrades gracefully.

**Phase 4 — Flip the flag + retire the string hack.**
- Set `multimodal.enabled: true` (or per-environment).
- **Delete** `buildUserContentBlocks`/`parseDataURI` (Anthropic) and
  `buildImageContent` (OpenAI). The typed path is now the only path.
- End-to-end test: console upload and a connector-sourced image both reach a
  vision model and produce a grounded response.

**Phase 5 — Telegram T1-5 (separate spec/PR, unblocked by Phases 1–4).**
- Telegram connector replaces the presence stopgap: `getFile` → download →
  `blob.Store.Put` → emit `Attachment` on the queued message.
- Tracked in [telegram-upgrades.md](telegram-upgrades.md) T1-5, not here.

---

## 9. Future Work

- **Speech-to-text:** voice notes → transcript text part (new STT provider
  interface; could reuse the `Parts` mechanism with an `audio` part type).
- **Document extraction / OCR:** PDFs and office docs → extracted text parts.
- **Vision-aware routing:** allow the router to prefer a vision-capable candidate
  when a turn carries images, without breaking deterministic routing guarantees.
- **Outbound media:** assistant-generated images/files back to the user.
- **Object-store blob backend:** swap `FilesystemStore` for S3/GCS behind the
  existing `blob.Store` interface for multi-node deployments.

---

## 10. Open Questions

1. **Inbox persistence size:** attachment *metadata* is small, but should
   `InboxItem.Attachments` live in a side table rather than inline JSON if counts
   grow? V1 assumes inline is fine (cap of 8/message).
2. **Blob lifecycle / GC:** when are chat attachment blobs deleted — with the chat,
   on retention expiry, or never? Needs a retention policy (defer to a storage spec).
3. **Idempotency with multipart:** the existing `idempotency_key` dedup must account
   for re-uploads; should a repeated key short-circuit *before* re-storing bytes?
4. **Per-provider image limits:** Anthropic/OpenAI cap image count and dimensions
   per request. Should assembly downscale, or reject, oversized images? V1 budgets
   by bytes only; dimension handling is open.

---

[docs INDEX](../INDEX.md) · [specs INDEX](INDEX.md) · [telegram-upgrades.md](telegram-upgrades.md)
