package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llm"
)

func TestUpsertDiscoveredModels(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	provider := "test-provider"

	// Test with 150 models (more than one batch of 100)
	count := 150
	models := make([]llm.ModelDescriptor, count)
	for i := 0; i < count; i++ {
		models[i] = llm.ModelDescriptor{
			Name:          fmt.Sprintf("model-%d", i),
			Size:          int64(i * 1000),
			Family:        "test-family",
			ParameterSize: "7B",
			QuantLevel:    "Q4_K_M",
			Digest:        fmt.Sprintf("digest-%d", i),
			ModifiedAt:    time.Now().UTC(),
		}
	}

	if err := UpsertDiscoveredModels(ctx, db, provider, models); err != nil {
		t.Fatalf("UpsertDiscoveredModels: %v", err)
	}

	// Verify count
	var actualCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM discovered_models WHERE provider = ?", provider).Scan(&actualCount)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if actualCount != count {
		t.Errorf("expected %d models, got %d", count, actualCount)
	}

	// Test update (upsert)
	models[0].Size = 999999
	if err := UpsertDiscoveredModels(ctx, db, provider, models[:1]); err != nil {
		t.Fatalf("UpsertDiscoveredModels update: %v", err)
	}

	var newSize int64
	err = db.QueryRowContext(ctx, "SELECT size_bytes FROM discovered_models WHERE provider = ? AND name = ?", provider, models[0].Name).Scan(&newSize)
	if err != nil {
		t.Fatalf("query updated size: %v", err)
	}
	if newSize != 999999 {
		t.Errorf("expected updated size 999999, got %d", newSize)
	}
}
