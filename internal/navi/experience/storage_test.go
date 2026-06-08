package experience

import (
	"context"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestLoadConfigurationSnapshotMergesStoredExperienceConfiguration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	if err := SaveModuleConfiguration(ctx, db, ConfigScopeGlobal, ConfigScopeSystem, ModuleConfig{
		ModuleID: "global_briefing",
		Scope:    ModuleScopeGlobal,
		Strength: 1,
		TraitContributions: map[string]float64{
			"structure_level": 0.82,
		},
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveModuleConfiguration(global): %v", err)
	}
	if err := SaveCoreIdentityConfiguration(ctx, db, "owner-1", map[string]float64{"warmth": 0.91}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveCoreIdentityConfiguration: %v", err)
	}
	summaryFirst := true
	if err := SaveOutputPreferencesConfiguration(ctx, db, "owner-1", OutputPreferences{PreferredLength: LengthShort, SummaryFirst: &summaryFirst}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveOutputPreferencesConfiguration: %v", err)
	}
	if err := SaveRelationshipProfileConfiguration(ctx, db, "owner-1", RelationshipProfile{
		TraitEstimates:     map[string]float64{"humor_playfulness": 0.05},
		Confidence:         0.44,
		PerTraitConfidence: map[string]float64{"humor_playfulness": 0.9},
		TotalSignalCount:   9,
	}, string(schema.StateKindInferred)); err != nil {
		t.Fatalf("SaveRelationshipProfileConfiguration: %v", err)
	}

	snapshot, err := LoadConfigurationSnapshot(ctx, db, "owner-1")
	if err != nil {
		t.Fatalf("LoadConfigurationSnapshot: %v", err)
	}
	if got := snapshot.CoreIdentity.Traits["warmth"]; got != 0.91 {
		t.Fatalf("expected stored warmth override, got %.2f", got)
	}
	if snapshot.OutputPreferences.PreferredLength != LengthShort {
		t.Fatalf("expected stored preferred length short, got %q", snapshot.OutputPreferences.PreferredLength)
	}
	if snapshot.OutputPreferences.SummaryFirst == nil || !*snapshot.OutputPreferences.SummaryFirst {
		t.Fatalf("expected stored summary_first=true, got %+v", snapshot.OutputPreferences)
	}
	if len(snapshot.PersonaModules) != 1 || snapshot.PersonaModules[0].ModuleID != "global_briefing" {
		t.Fatalf("expected stored module registry entry, got %+v", snapshot.PersonaModules)
	}
	if snapshot.PersonaModules[0].Version != "v1" || snapshot.PersonaModules[0].Kind != ModuleKindBundle {
		t.Fatalf("expected default module version/kind metadata, got %+v", snapshot.PersonaModules[0])
	}
	if snapshot.RelationshipProfile.Confidence != 0.44 {
		t.Fatalf("expected stored relationship confidence, got %.2f", snapshot.RelationshipProfile.Confidence)
	}
}

func TestUpdateRelationshipProfileAppliesSignalsAndComputesConfidence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	profile, deltas := UpdateRelationshipProfile(RelationshipProfile{}, []schema.PreferenceSignal{
		{
			Trait:          "verbosity",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			CreatedAt:      now,
		},
		{
			Trait:          "verbosity",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			CreatedAt:      now,
		},
		{
			Trait:          "verbosity",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			CreatedAt:      now,
		},
	}, now)

	if profile.TraitSignalCount["verbosity"] != 3 {
		t.Fatalf("expected trait signal count 3, got %+v", profile.TraitSignalCount)
	}
	if got := profile.TraitEstimates["verbosity"]; got >= traitDefinitions["verbosity"].DefaultValue {
		t.Fatalf("expected verbosity to move toward concise target, got %.2f", got)
	}
	if profile.Confidence <= 0 {
		t.Fatalf("expected profile confidence to increase, got %.2f", profile.Confidence)
	}
	if len(deltas) != 1 || deltas[0].Trait != "verbosity" {
		t.Fatalf("expected verbosity adaptation delta, got %+v", deltas)
	}
	if !deltas[0].Applied {
		t.Fatalf("expected verbosity adaptation to apply after 3 signals, got %+v", deltas[0])
	}
	if deltas[0].Persisted {
		t.Fatalf("did not expect verbosity adaptation to persist before 5 signals, got %+v", deltas[0])
	}
}

func TestMaybeRecordExperienceSnapshotRecordsSessionStartAndMaterialDelta(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	engine := NewEngine(nil)
	rendered1, err := engine.Build(ctx, DefaultStandardProfile(), BuildRequest{
		ChatID:       "sess-1",
		Mode:            "navi",
		LastUserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Build(rendered1): %v", err)
	}
	recorded, err := MaybeRecordExperienceSnapshot(ctx, db, rendered1, SnapshotRecordOptions{
		ChatID:      "sess-1",
		OwnerID:        "owner-1",
		ExperienceMode: "navi",
		SessionStart:   true,
	})
	if err != nil {
		t.Fatalf("MaybeRecordExperienceSnapshot(session_start): %v", err)
	}
	if !recorded {
		t.Fatal("expected initial session_start snapshot to be recorded")
	}

	recorded, err = MaybeRecordExperienceSnapshot(ctx, db, rendered1, SnapshotRecordOptions{
		ChatID:      "sess-1",
		OwnerID:        "owner-1",
		ExperienceMode: "navi",
	})
	if err != nil {
		t.Fatalf("MaybeRecordExperienceSnapshot(no_change): %v", err)
	}
	if recorded {
		t.Fatal("expected identical rendered control not to produce a new snapshot")
	}

	rendered2, err := engine.Build(ctx, DefaultStandardProfile(), BuildRequest{
		ChatID:       "sess-1",
		Mode:            "navi",
		LastUserMessage: "hello",
		ExplicitSessionOverrides: ExplicitTurnInput{
			TraitOverrides: map[string]float64{"verbosity": 0.20},
		},
	})
	if err != nil {
		t.Fatalf("Build(rendered2): %v", err)
	}
	recorded, err = MaybeRecordExperienceSnapshot(ctx, db, rendered2, SnapshotRecordOptions{
		ChatID:      "sess-1",
		OwnerID:        "owner-1",
		ExperienceMode: "navi",
	})
	if err != nil {
		t.Fatalf("MaybeRecordExperienceSnapshot(material_delta): %v", err)
	}
	if !recorded {
		t.Fatal("expected material_delta snapshot to be recorded")
	}

	events, err := store.EventsByCorrelationID(ctx, db, "sess-1")
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 snapshot events, got %d", len(events))
	}

	firstPayload, err := decodeExperienceSnapshotPayload(events[0].Payload)
	if err != nil {
		t.Fatalf("decodeExperienceSnapshotPayload(first): %v", err)
	}
	secondPayload, err := decodeExperienceSnapshotPayload(events[1].Payload)
	if err != nil {
		t.Fatalf("decodeExperienceSnapshotPayload(second): %v", err)
	}
	if firstPayload.Trigger != SnapshotTriggerSessionStart {
		t.Fatalf("expected first trigger %q, got %q", SnapshotTriggerSessionStart, firstPayload.Trigger)
	}
	if secondPayload.Trigger != SnapshotTriggerMaterialDelta {
		t.Fatalf("expected second trigger %q, got %q", SnapshotTriggerMaterialDelta, secondPayload.Trigger)
	}
	if secondPayload.CumulativeTraitDelta <= snapshotMaterialDeltaThreshold {
		t.Fatalf("expected cumulative trait delta > %.2f, got %.4f", snapshotMaterialDeltaThreshold, secondPayload.CumulativeTraitDelta)
	}
}

func TestRecordSessionEndExperienceSnapshotReusesLatestState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	engine := NewEngine(nil)
	rendered, err := engine.Build(ctx, DefaultStandardProfile(), BuildRequest{
		ChatID:       "sess-end",
		Mode:            "navi",
		LastUserMessage: "wrap this up",
	})
	if err != nil {
		t.Fatalf("Build(rendered): %v", err)
	}
	recorded, err := MaybeRecordExperienceSnapshot(ctx, db, rendered, SnapshotRecordOptions{
		ChatID:      "sess-end",
		OwnerID:        "owner-1",
		ExperienceMode: "navi",
		SessionStart:   true,
	})
	if err != nil {
		t.Fatalf("MaybeRecordExperienceSnapshot(session_start): %v", err)
	}
	if !recorded {
		t.Fatal("expected initial session_start snapshot to be recorded")
	}

	recorded, err = RecordSessionEndExperienceSnapshot(ctx, db, "sess-end", "owner-1", "navi")
	if err != nil {
		t.Fatalf("RecordSessionEndExperienceSnapshot: %v", err)
	}
	if !recorded {
		t.Fatal("expected session_end snapshot to be recorded")
	}

	events, err := store.EventsByCorrelationID(ctx, db, "sess-end")
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 snapshot events, got %d", len(events))
	}

	startPayload, err := decodeExperienceSnapshotPayload(events[0].Payload)
	if err != nil {
		t.Fatalf("decodeExperienceSnapshotPayload(start): %v", err)
	}
	endPayload, err := decodeExperienceSnapshotPayload(events[1].Payload)
	if err != nil {
		t.Fatalf("decodeExperienceSnapshotPayload(end): %v", err)
	}
	if endPayload.Trigger != SnapshotTriggerSessionEnd {
		t.Fatalf("expected end trigger %q, got %q", SnapshotTriggerSessionEnd, endPayload.Trigger)
	}
	if endPayload.SourceStateID != startPayload.SourceStateID {
		t.Fatalf("expected session_end SourceStateID %q, got %q", startPayload.SourceStateID, endPayload.SourceStateID)
	}
	if endPayload.CompiledPayloadID != startPayload.CompiledPayloadID {
		t.Fatalf("expected session_end CompiledPayloadID %q, got %q", startPayload.CompiledPayloadID, endPayload.CompiledPayloadID)
	}
	if endPayload.EffectiveStateJSON != startPayload.EffectiveStateJSON {
		t.Fatal("expected session_end to reuse latest effective state snapshot")
	}
	if endPayload.CompiledPayloadJSON != startPayload.CompiledPayloadJSON {
		t.Fatal("expected session_end to reuse latest compiled payload snapshot")
	}
}
