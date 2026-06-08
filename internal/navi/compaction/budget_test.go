package compaction

import "testing"

func TestCompaction_BudgetManager_ComputesUsableBudget(t *testing.T) {
	bm := NewBudgetManager(nil, BudgetConfig{})
	s := bm.Snapshot(1000, 200, 100, []Message{{Content: "hello"}})
	if s.UsableTokens != 700 {
		t.Fatalf("usable=%d want 700", s.UsableTokens)
	}
}

func TestCompaction_Thresholds_SoftHardEmergency_AreOrdered(t *testing.T) {
	bm := NewBudgetManager(nil, BudgetConfig{})
	s := bm.Snapshot(1000, 100, 100, nil)
	if !(s.SoftThreshold < s.HardThreshold && s.HardThreshold < s.EmergencyThreshold) {
		t.Fatalf("thresholds not ordered: %+v", s)
	}
}

func TestCompaction_UnderSoftThreshold_IsNoOp(t *testing.T) {
	bm := NewBudgetManager(nil, BudgetConfig{})
	s := bm.Snapshot(1000, 100, 100, []Message{{Content: "short"}})
	if trigger := bm.TriggerClass(s); trigger != "" {
		t.Fatalf("unexpected trigger %s", trigger)
	}
}
