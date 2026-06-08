package navi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCompaction_HardThreshold_RunsBeforeModelExecution guards the ordering
// invariant: compaction must run before the model request is compiled, so the
// model never sees an over-budget context. This is a source-level guard because
// the ordering is structural in runtime_executor.go.
func TestCompaction_HardThreshold_RunsBeforeModelExecution(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	content, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "runtime_executor.go"))
	if err != nil {
		t.Fatalf("read runtime executor: %v", err)
	}
	txt := string(content)
	compactionIdx := strings.Index(txt, "runCompactionBeforeModel")
	compileIdx := strings.Index(txt, "compileRunModelRequest")
	if compactionIdx < 0 || compileIdx < 0 {
		t.Fatalf("expected compaction + model-compile hooks in runtime_executor.go")
	}
	if compactionIdx > compileIdx {
		t.Fatalf("compaction hook appears after model request compile")
	}
}

// TestStructuredCompaction_DisabledWithoutChatStore verifies the safe defaults
// when no compaction-capable chat store is configured: structured compaction is
// reported disabled and the continuity block is empty (no panic on nil store).
func TestStructuredCompaction_DisabledWithoutChatStore(t *testing.T) {
	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
	})

	if loop.isStructuredCompactionEnabled() {
		t.Fatal("expected structured compaction to be disabled when no chat store is configured")
	}
	if block := loop.chatCompactionBlock(context.Background(), "chat-1"); strings.TrimSpace(block) != "" {
		t.Fatalf("expected empty continuity block without a chat memory store, got %q", block)
	}
	// runCompactionBeforeModel must be a no-op (not panic) when compaction is disabled.
	loop.runCompactionBeforeModel(context.Background(), "chat-1", "hello")
}
