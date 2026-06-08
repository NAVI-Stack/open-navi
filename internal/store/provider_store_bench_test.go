package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/llm"
)

func BenchmarkUpsertDiscoveredModels(b *testing.B) {
	db, err := Open(":memory:")
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		b.Fatalf("CreateTables: %v", err)
	}

	provider := "test-provider"

	for _, count := range []int{10, 100, 1000} {
		models := make([]llm.ModelDescriptor, count)
		for i := 0; i < count; i++ {
			models[i] = llm.ModelDescriptor{
				Name:          fmt.Sprintf("model-%d", i),
				Size:          1024 * 1024,
				Family:        "test-family",
				ParameterSize: "7B",
				QuantLevel:    "Q4_K_M",
				Digest:        fmt.Sprintf("digest-%d", i),
				ModifiedAt:    time.Now(),
			}
		}

		b.Run(fmt.Sprintf("Count-%d", count), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				err := UpsertDiscoveredModels(ctx, db, provider, models)
				if err != nil {
					b.Fatalf("UpsertDiscoveredModels: %v", err)
				}
			}
		})
	}
}
