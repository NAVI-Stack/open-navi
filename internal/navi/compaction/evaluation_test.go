package compaction

import (
	"context"
	"strings"
	"testing"
)

type evaluationScenario struct {
	name           string
	class          string
	input          RunInput
	initialMem     ChatMemory
	repeat         int
	failCheckpoint bool
	wantErr        bool
	assert         func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory)
}

func TestEvaluationHarness_ScenariosProduceInspectableEvidence(t *testing.T) {
	scenarios := []evaluationScenario{
		{
			name:  "social continuity",
			class: "simple_social_continuity",
			input: RunInput{
				ChatID:             "social-1",
				Messages:              []Message{{ID: "m0", Role: "user", Content: strings.Repeat("We talked about coffee and training. ", 220)}, {ID: "m1", Role: "navi", Content: strings.Repeat("I kept the tone warm and supportive. ", 120)}, {ID: "m2", Role: "user", Content: "We were talking about marathon pacing"}, {ID: "m3", Role: "navi", Content: "You were leaning toward shorter intervals"}, {ID: "m4", Role: "user", Content: "Let's pick this up tomorrow"}},
				NewInput:              "Keep the conversation warm and remember the marathon pacing thread.",
				MaxContextTokens:      180,
				ReservedOutputTokens:  40,
				ReservedToolHeadroom:  30,
				PermanentInstructions: []string{"Be warm and grounded."},
			},
			assert: func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory) {
				if evidence.Checkpoint == nil {
					t.Fatal("expected social scenario to compact successfully")
				}
				if evidence.ChatFrame.PrimaryObjective == "" {
					t.Fatal("expected social objective continuity")
				}
				if len(evidence.RehydrationAfterTrim.Dropped) == 0 {
					t.Fatal("expected social scenario to demonstrate trimming under pressure")
				}
				if got := evidence.RehydrationAfterTrim.Metadata.DroppedSections[0].Reason; got != "trimmed_support_context_first" {
					t.Fatalf("expected support context to drop first, got %q", got)
				}
			},
		},
		{
			name:  "technical planning",
			class: "technical_planning",
			input: RunInput{
				ChatID: "plan-1",
				Messages: []Message{
					{ID: "m0", Role: "user", Content: strings.Repeat("We compared rollout plans and architecture tradeoffs. ", 200)},
					{ID: "m1", Role: "user", RunID: "run-arch", Content: "[task:run-arch] Finalize compaction architecture"},
					{ID: "m2", Role: "navi", RunID: "run-arch", Content: "Plan: document runtime snapshots\nDependency: ADR signoff\nNext step: update evaluation docs", MessageKind: "task_active"},
					{ID: "m3", Role: "user", RunID: "run-rollout", Content: "[task:run-rollout] Prepare rollout checklist"},
					{ID: "m4", Role: "navi", RunID: "run-rollout", Content: "BLOCKER: waiting on operator review\nNext step: get review signoff", MessageKind: "task_blocked"},
					{ID: "m5", Role: "user", Content: "tail"},
					{ID: "m6", Role: "navi", Content: "tail2"},
				},
				RuntimeSnapshots: []RunSnapshot{
					{RunID: "run-arch", Status: "running", CurrentPhase: "execute", LatestCheckpointID: "cp-arch", Scratchpad: map[string]string{"goal": "Finalize compaction architecture", "current_step": "update evaluation docs"}},
					{RunID: "run-rollout", Status: "waiting_for_proposal", CurrentPhase: "validate_govern", LatestCheckpointID: "cp-rollout", PendingProposalID: "proposal-rollout", PendingProposalReason: "needs operator review", Scratchpad: map[string]string{"goal": "Prepare rollout checklist", "current_step": "get review signoff"}},
				},
				MaxContextTokens:     420,
				ReservedOutputTokens: 60,
				ReservedToolHeadroom: 40,
			},
			assert: func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory) {
				if evidence.Checkpoint == nil {
					t.Fatal("expected planning scenario to compact successfully")
				}
				if len(mem.TaskFrames) < 2 {
					t.Fatalf("expected multiple active task frames, got %+v", mem.TaskFrames)
				}
				if mem.TaskFrames[0].TaskID != "run-arch" || mem.TaskFrames[1].TaskID != "run-rollout" {
					t.Fatalf("unexpected task identities: %+v", mem.TaskFrames)
				}
				if evidence.InspectionView.TaskSurvival == nil || len(evidence.InspectionView.TaskSurvival) < 2 {
					t.Fatalf("expected inspectable task survival summaries, got %+v", evidence.InspectionView.TaskSurvival)
				}
			},
		},
		{
			name:  "coding debugging",
			class: "coding_debugging",
			input: RunInput{
				ChatID: "debug-1",
				Messages: []Message{
					{ID: "m0", Role: "user", Content: strings.Repeat("We inspected logs and stack traces from panic output. ", 210)},
					{ID: "m1", Role: "navi", RunID: "run-debug", MessageKind: "tool_call", Content: "Calling tool read_file for service.go"},
					{ID: "m2", Role: "navi", RunID: "run-debug", MessageKind: "tool_result", Content: "Tool result: nil pointer at internal/navi/compaction/service.go:88"},
					{ID: "m3", Role: "navi", RunID: "run-debug", MessageKind: "failure_open", Content: "[failure:run-debug] tests failed"},
					{ID: "m4", Role: "navi", RunID: "run-debug", MessageKind: "recovery_active", Content: "Recovery: retrying with smaller harness fixture"},
					{ID: "m5", Role: "user", Content: "tail"},
				},
				RuntimeSnapshots:     []RunSnapshot{{RunID: "run-debug", Status: "failed", CurrentPhase: "execute", LatestCheckpointID: "cp-debug", Scratchpad: map[string]string{"goal": "Fix compaction panic", "current_step": "re-run failing scenario"}}},
				FailureRefs:          []string{"run-debug"},
				MaxContextTokens:     340,
				ReservedOutputTokens: 50,
				ReservedToolHeadroom: 40,
			},
			assert: func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory) {
				if evidence.Checkpoint != nil {
					t.Fatal("expected coding/debugging scenario to refuse an unsafe cut")
				}
				if evidence.Selection.StopReason == nil || (!strings.Contains(evidence.InspectionView.StopReason, "tool") && !strings.Contains(evidence.InspectionView.StopReason, "failure") && !strings.Contains(evidence.InspectionView.StopReason, "recovery")) {
					t.Fatalf("expected explainable protected bundle stop reason, got %+v / %q", evidence.Selection.StopReason, evidence.InspectionView.StopReason)
				}
				if len(evidence.RehydrationAfterTrim.Sections) == 0 {
					t.Fatal("expected usable degraded rehydration output")
				}
			},
		},
		{
			name:  "proposal workflow",
			class: "proposal_approval_workflow",
			input: RunInput{
				ChatID: "proposal-1",
				Messages: []Message{
					{ID: "m0", Role: "user", Content: strings.Repeat("We discussed the governed action and the files it would touch. ", 180)},
					{ID: "m1", Role: "navi", RunID: "run-proposal", MessageKind: "proposal_open", Content: "[proposal:proposal-9] Approval required to edit compose.yml"},
					{ID: "m2", Role: "user", Content: "tail"},
					{ID: "m3", Role: "navi", Content: "tail2"},
				},
				RuntimeSnapshots:     []RunSnapshot{{RunID: "run-proposal", Status: "waiting_for_proposal", CurrentPhase: "validate_govern", LatestCheckpointID: "cp-proposal", PendingProposalID: "proposal-9", PendingProposalReason: "edit compose.yml", Scratchpad: map[string]string{"goal": "Prepare governed edit", "current_step": "await user approval"}}},
				ProposalRefs:         []string{"proposal-9"},
				MaxContextTokens:     320,
				ReservedOutputTokens: 50,
				ReservedToolHeadroom: 40,
			},
			assert: func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory) {
				if evidence.Checkpoint != nil {
					t.Fatal("expected proposal scenario to refuse an unsafe cut")
				}
				joined := strings.Join(evidence.RehydrationAfterTrim.Sections, "\n\n")
				if !strings.Contains(joined, "## Active Proposals") || !strings.Contains(joined, "proposal-9") {
					t.Fatalf("expected authoritative proposal refs to remain visible, got %q", joined)
				}
				if evidence.Selection.StopReason == nil || !strings.Contains(evidence.InspectionView.StopReason, "proposal") {
					t.Fatalf("expected explainable proposal boundary stop, got %q", evidence.InspectionView.StopReason)
				}
			},
		},
		{
			name:  "execution recovery",
			class: "multi_step_execution_recovery",
			input: RunInput{
				ChatID: "recovery-1",
				Messages: []Message{
					{ID: "m0", Role: "user", Content: strings.Repeat("We iterated on the execution and recovery plan. ", 200)},
					{ID: "m1", Role: "navi", RunID: "run-exec", Content: "updated file report.md", MessageKind: "artifact_mutation"},
					{ID: "m2", Role: "navi", RunID: "run-exec", Content: "[failure:run-exec] command failed", MessageKind: "failure_open"},
					{ID: "m3", Role: "navi", RunID: "run-exec", Content: "Recovery: retrying run-exec", MessageKind: "recovery_active"},
					{ID: "m4", Role: "user", Content: "tail"},
				},
				RuntimeSnapshots: []RunSnapshot{{
					RunID:              "run-exec",
					Status:             "failed",
					CurrentPhase:       "execute",
					LatestCheckpointID: "cp-exec",
					MainArtifactID:     "artifact:report",
					ArtifactIDs:        []string{"artifact:report"},
					Scratchpad:         map[string]string{"goal": "Repair report generation", "current_step": "retry command"},
				}},
				FailureRefs:          []string{"run-exec"},
				MaxContextTokens:     320,
				ReservedOutputTokens: 50,
				ReservedToolHeadroom: 40,
			},
			assert: func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory) {
				if evidence.Checkpoint != nil {
					t.Fatal("expected execution/recovery scenario to refuse an unsafe cut")
				}
				if evidence.Selection.StopReason == nil || evidence.Selection.StopReason.Kind == "" {
					t.Fatalf("expected boundary stop reason for recovery scenario, got %+v", evidence.Selection)
				}
				if len(evidence.RehydrationAfterTrim.Sections) == 0 {
					t.Fatal("expected usable degraded rehydration output")
				}
			},
		},
		{
			name:   "mixed long running",
			class:  "mixed_long_running_session",
			repeat: 2,
			input: RunInput{
				ChatID: "mixed-1",
				Messages: []Message{
					{ID: "m0", Role: "user", Content: strings.Repeat("We alternated between chat, planning, and execution. ", 220)},
					{ID: "m1", Role: "user", RunID: "run-a", Content: "[task:run-a] Finish the design note"},
					{ID: "m2", Role: "navi", RunID: "run-a", Content: "Plan: tighten the rubric\nNext step: write evaluation harness"},
					{ID: "m3", Role: "user", RunID: "run-b", Content: "[task:run-b] Validate the runtime path"},
					{ID: "m4", Role: "navi", RunID: "run-b", Content: "BLOCKER: awaiting approval\nNext step: resolve proposal"},
					{ID: "m5", Role: "user", Content: "tail"},
					{ID: "m6", Role: "navi", Content: "tail2"},
				},
				RuntimeSnapshots: []RunSnapshot{
					{RunID: "run-a", Status: "running", CurrentPhase: "execute", LatestCheckpointID: "cp-a", Scratchpad: map[string]string{"goal": "Finish the design note", "current_step": "write evaluation harness"}},
					{RunID: "run-b", Status: "waiting_for_proposal", CurrentPhase: "validate_govern", LatestCheckpointID: "cp-b", PendingProposalID: "proposal-b", PendingProposalReason: "runtime path edit", Scratchpad: map[string]string{"goal": "Validate the runtime path", "current_step": "resolve proposal"}},
				},
				MaxContextTokens:     420,
				ReservedOutputTokens: 60,
				ReservedToolHeadroom: 40,
			},
			assert: func(t *testing.T, evidence ScenarioEvidence, mem ChatMemory) {
				if evidence.Checkpoint == nil {
					t.Fatal("expected mixed long-running scenario to compact successfully")
				}
				if mem.ChatFrame.FrameVersion < 2 {
					t.Fatalf("expected continuity to remain stable across epochs, got frame version %d", mem.ChatFrame.FrameVersion)
				}
				if len(mem.TaskFrames) < 2 || mem.TaskFrames[0].TaskID != "run-a" || mem.TaskFrames[1].TaskID != "run-b" {
					t.Fatalf("expected stable task identities across epochs, got %+v", mem.TaskFrames)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			evidence, mem := runEvaluationScenario(t, scenario)
			assertBaselineEvidence(t, evidence, scenario.wantErr)
			scenario.assert(t, evidence, mem)
		})
	}
}

func TestEvaluationHarness_DegradationQuality_OnCheckpointFailure(t *testing.T) {
	scenario := evaluationScenario{
		name:           "checkpoint failure degradation",
		class:          "degradation_quality",
		failCheckpoint: true,
		wantErr:        true,
		input: RunInput{
			ChatID:            "fail-1",
			Messages:             []Message{{ID: "m0", Role: "user", Content: strings.Repeat("older context ", 240)}, {ID: "m1", Role: "navi", Content: "older reply"}, {ID: "m2", Role: "user", Content: "tail"}, {ID: "m3", Role: "navi", Content: "tail2"}},
			NewInput:             "Continue the session without surfacing a compaction-specific error.",
			MaxContextTokens:     220,
			ReservedOutputTokens: 40,
			ReservedToolHeadroom: 30,
		},
	}

	evidence, mem := runEvaluationScenario(t, scenario)
	if evidence.Error == "" {
		t.Fatal("expected captured checkpoint failure")
	}
	if mem.ChatID == "" || mem.ChatFrame.ChatID == "" {
		t.Fatalf("expected structured continuity surfaces to remain inspectable after failure, got %+v", mem)
	}
	if len(evidence.RehydrationBeforeTrim.Sections) == 0 {
		t.Fatal("expected fallback evidence to show usable rehydration surfaces")
	}
}

func runEvaluationScenario(t *testing.T, scenario evaluationScenario) (ScenarioEvidence, ChatMemory) {
	t.Helper()

	mem := &memStore{mem: scenario.initialMem}
	cp := &cpStore{fail: scenario.failCheckpoint}
	svc := Service{
		Budget:     NewBudgetManager(nil, BudgetConfig{}),
		Selector:   SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1},
		Rehydrator: Rehydrator{},
		Memory:     mem,
		Checkpoint: cp,
	}

	repeat := scenario.repeat
	if repeat <= 0 {
		repeat = 1
	}

	var out RunOutput
	var err error
	for i := 0; i < repeat; i++ {
		out, err = svc.Run(context.Background(), scenario.input)
		if err != nil {
			break
		}
	}

	evidence := BuildScenarioEvidence(scenario.name, scenario.class, scenario.input, mem.mem, out, err)
	return evidence, mem.mem
}

func assertBaselineEvidence(t *testing.T, evidence ScenarioEvidence, wantErr bool) {
	t.Helper()

	if evidence.ScenarioName == "" || evidence.ScenarioClass == "" {
		t.Fatalf("expected scenario metadata, got %+v", evidence)
	}
	if evidence.Budget.UsableTokens <= 0 {
		t.Fatalf("expected budget snapshot, got %+v", evidence.Budget)
	}
	if wantErr {
		return
	}
	if evidence.TriggerReason == "" {
		t.Fatalf("expected explainable trigger reason, got %+v", evidence)
	}
	if evidence.Checkpoint != nil && evidence.CheckpointView.CompactedRange == "" {
		t.Fatalf("expected checkpoint compacted range, got %+v", evidence.CheckpointView)
	}
	if evidence.Checkpoint != nil && (evidence.SelectedSpan.StartMessageID == "" || len(evidence.SelectedSpan.SourceMessageIDs) == 0) {
		t.Fatalf("expected source traceability, got %+v", evidence.SelectedSpan)
	}
	if len(evidence.RehydrationBeforeTrim.Sections) == 0 || len(evidence.RehydrationAfterTrim.Sections) == 0 {
		t.Fatalf("expected before/after rehydration evidence, got before=%+v after=%+v", evidence.RehydrationBeforeTrim, evidence.RehydrationAfterTrim)
	}
	if evidence.Checkpoint == nil && evidence.InspectionView.StopReason == "" {
		t.Fatalf("expected explainable no-cut reason when compaction is blocked, got %+v", evidence)
	}
	if len(evidence.RehydrationAfterTrim.Metadata.DroppedSections) > 0 && len(evidence.RehydrationAfterTrim.Dropped) == 0 {
		t.Fatalf("expected dropped labels to match dropped metadata, got %+v", evidence.RehydrationAfterTrim)
	}
}
