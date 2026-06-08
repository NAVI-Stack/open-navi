package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	coreskill "github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/store"
)

type SkillResult = coreskill.SkillResult

var GetInternalHandler = coreskill.GetInternalHandler

type documentKnowledgeTestLLM struct {
	content string
}

func (d *documentKnowledgeTestLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return &llm.Response{Content: d.content}, nil
}

func (d *documentKnowledgeTestLLM) Name() string { return "document-knowledge-test" }

func TestDocumentKnowledgeBridgeExtractsSimplePDF(t *testing.T) {
	pythonExe, err := testPythonExecutable()
	if err != nil {
		t.Fatalf("resolvePythonExecutable: %v", err)
	}

	bridgePath := filepath.Join("..", "..", "..", "plugins", "document-knowledge", "skills", "document-knowledge", "bridge.py")
	pdfPath := filepath.Join(t.TempDir(), "sample.pdf")
	pdfContent := `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT
/F1 12 Tf
72 72 Td
(Hello PDF World) Tj
ET
endstream
endobj
trailer
<< /Root 1 0 R >>
%%EOF`
	if err := os.WriteFile(pdfPath, []byte(pdfContent), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	input, _ := json.Marshal(map[string]any{"path": pdfPath})
	cmd := exec.Command(pythonExe, bridgePath)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run bridge: %v: %s", err, strings.TrimSpace(stderr.String()))
	}

	var result SkillResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode skill result: %v", err)
	}
	if result.Status != SkillResultSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	payloadBytes, _ := json.Marshal(result.Output)
	var payload DocumentExtraction
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("decode extraction payload: %v", err)
	}
	if payload.DocumentType != "pdf" {
		t.Fatalf("expected pdf document type, got %q", payload.DocumentType)
	}
	if !strings.Contains(payload.Text, "Hello PDF World") {
		t.Fatalf("expected extracted PDF text, got %q", payload.Text)
	}
}

func testPythonExecutable() (string, error) {
	for _, name := range []string{"python", "py", "python3"} {
		if path, err := exec.LookPath(name); err == nil {
			if pythonExecutableUsable(path) {
				return path, nil
			}
		}
	}
	return "", os.ErrNotExist
}

func pythonExecutableUsable(path string) bool {
	cmd := exec.Command(path, "-V")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return cmd.Run() == nil
}

func TestDocumentKnowledgeIngestPersistsFactsAndMemory(t *testing.T) {
	db := store.InitTestDB(t)
	workspaceDir := t.TempDir()
	RegisterDocumentKnowledgeHandlers("document-knowledge", DocumentKnowledgeHandlerConfig{
		DB:           db,
		LLM:          &documentKnowledgeTestLLM{content: `{"summary":"The document says NAVI should keep the daemon in Go.","facts":[{"key":"daemon_language","value":"Use Go for the daemon","category":"technical_context"}]}`},
		Model:        "test-model",
		WorkspaceDir: workspaceDir,
		ResolveOwnerID: func(ctx context.Context, chatID string) string {
			if chatID == "sess-doc" {
				return "owner-1"
			}
			return ""
		},
	})

	origExtractor := documentExtractor
	documentExtractor = func(ctx context.Context, entry *SkillEntry, resolvedPath string) (DocumentExtraction, error) {
		return DocumentExtraction{
			Path:         resolvedPath,
			DocumentType: "text",
			Text:         "NAVI should use Go for the daemon and keep SQLite as the local store.",
			TextPreview:  "NAVI should use Go for the daemon...",
			Entities: []DocumentEntity{
				{Text: "NAVI", Type: "proper_noun"},
				{Text: "Go", Type: "proper_noun"},
			},
		}, nil
	}
	defer func() { documentExtractor = origExtractor }()

	handler, ok := GetInternalHandler("document-knowledge", "ingest_document")
	if !ok {
		t.Fatal("expected ingest_document handler to be registered")
	}

	entry := &SkillEntry{
		Skill: Skill{BaseDir: filepath.Join("..", "..", "..", "plugins", "document-knowledge", "skills", "document-knowledge")},
		Spec: &OSS27Spec{
			SkillID: "document-knowledge",
		},
	}

	ctx := WithExecutionContext(context.Background(), ExecutionContext{ChatID: "sess-doc"})
	payload, err := handler(ctx, entry, &Interface{Name: "ingest_document"}, map[string]any{"path": "notes.txt"})
	if err != nil {
		t.Fatalf("ingest_document: %v", err)
	}
	result, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", payload)
	}
	if !strings.Contains(result["summary"].(string), "daemon in Go") {
		t.Fatalf("unexpected summary: %v", result["summary"])
	}

	facts, err := store.ListFacts(context.Background(), db, "owner", "owner-1", false, 10, false)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected one owner fact, got %d", len(facts))
	}
	if facts[0].Key != "daemon_language" {
		t.Fatalf("expected daemon_language fact, got %q", facts[0].Key)
	}

	memories, err := store.ListMemories(context.Background(), db, "chat", "sess-doc", 10)
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected one chat memory, got %d", len(memories))
	}
	if memories[0].Source != "document_ingestion" {
		t.Fatalf("expected document_ingestion source, got %q", memories[0].Source)
	}
	if memories[0].Significance != "medium" {
		t.Fatalf("expected medium significance, got %q", memories[0].Significance)
	}

	scopes, ok := result["knowledge_scopes"].([]string)
	if !ok || len(scopes) != 2 {
		t.Fatalf("expected chat and owner scopes, got %v", result["knowledge_scopes"])
	}
	if scopes[0] != "chat:sess-doc" || scopes[1] != "owner:owner-1" {
		t.Fatalf("expected chat and owner scopes, got %v", scopes)
	}
}
