package main

import (
	"os"
	"testing"
)

func TestParseKnowledgeEmbedVector(t *testing.T) {
	vector, err := parseKnowledgeEmbedVector(`{"status":"success","payload":{"vector":[0.1,0.2,0.3]}}`)
	if err != nil {
		t.Fatalf("parseKnowledgeEmbedVector: %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("expected vector length 3, got %d", len(vector))
	}
	if vector[0] != 0.1 || vector[1] != 0.2 || vector[2] != 0.3 {
		t.Fatalf("unexpected vector: %v", vector)
	}
}

func TestSyncKnowledgeEmbedderEnvSetsOpenAIKeyWhenMissing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	syncKnowledgeEmbedderEnv("test-key")
	if got := os.Getenv("OPENAI_API_KEY"); got != "test-key" {
		t.Fatalf("expected OPENAI_API_KEY to be set, got %q", got)
	}
}

func TestSyncKnowledgeEmbedderEnvUpdatesExistingOpenAIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "existing-key")
	syncKnowledgeEmbedderEnv("new-key")
	if got := os.Getenv("OPENAI_API_KEY"); got != "new-key" {
		t.Fatalf("expected OPENAI_API_KEY to update, got %q", got)
	}
}
