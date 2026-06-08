package connectors

import (
	"fmt"
	"testing"
)

func TestDiagCollector_PushAndList(t *testing.T) {
	d := NewDiagCollector()
	d.Push(DiagInfo, "test", "hello", nil)
	d.Push(DiagWarning, "test", "warn", fmt.Errorf("oops"))

	items := d.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Level != DiagInfo {
		t.Errorf("items[0].Level = %q, want %q", items[0].Level, DiagInfo)
	}
	if items[1].Error != "oops" {
		t.Errorf("items[1].Error = %q, want %q", items[1].Error, "oops")
	}
}

func TestDiagCollector_RingOverflow(t *testing.T) {
	d := NewDiagCollector()
	for i := 0; i < diagRingSize+10; i++ {
		d.Push(DiagInfo, "test", fmt.Sprintf("msg-%d", i), nil)
	}

	items := d.List()
	if len(items) != diagRingSize {
		t.Fatalf("expected %d items, got %d", diagRingSize, len(items))
	}
	// Oldest should be msg-10 (first 10 were overwritten)
	if items[0].Message != "msg-10" {
		t.Errorf("oldest = %q, want %q", items[0].Message, "msg-10")
	}
	// Newest should be msg-109
	if items[diagRingSize-1].Message != fmt.Sprintf("msg-%d", diagRingSize+9) {
		t.Errorf("newest = %q, want %q", items[diagRingSize-1].Message, fmt.Sprintf("msg-%d", diagRingSize+9))
	}
}

func TestDiagCollector_EmptyList(t *testing.T) {
	d := NewDiagCollector()
	items := d.List()
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}
