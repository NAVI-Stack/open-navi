package navi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/llm"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

type workflowEventStore struct {
	events []schema.Event
}

func (s *workflowEventStore) AcceptMessage(context.Context, string, *naviruntime.InboxItem, string) (*naviruntime.InboxItem, error) {
	return nil, nil
}
func (s *workflowEventStore) AcceptSignal(context.Context, *naviruntime.InboxItem) (*naviruntime.InboxItem, error) {
	return nil, nil
}
func (s *workflowEventStore) ListPendingRuntimeSessions(context.Context) ([]string, error) {
	return nil, nil
}
func (s *workflowEventStore) ListPendingItems(context.Context, string, int) ([]naviruntime.InboxItem, error) {
	return nil, nil
}
func (s *workflowEventStore) MarkInboxConsumed(context.Context, string, string) error { return nil }
func (s *workflowEventStore) PromoteDeferredItems(context.Context, string, int) (int, error) {
	return 0, nil
}
func (s *workflowEventStore) CreateRun(context.Context, *naviruntime.RunState) error { return nil }
func (s *workflowEventStore) UpdateRun(context.Context, *naviruntime.RunState) error { return nil }
func (s *workflowEventStore) GetLatestRun(context.Context, string) (*naviruntime.RunState, error) {
	return nil, nil
}
func (s *workflowEventStore) GetRun(context.Context, string) (*naviruntime.RunState, error) {
	return nil, nil
}
func (s *workflowEventStore) GetPausedRun(context.Context, string) (*naviruntime.RunState, error) {
	return nil, nil
}
func (s *workflowEventStore) LookupRuntimeSessionKind(context.Context, string) (schema.RuntimeSessionKind, error) {
	return schema.RuntimeSessionKindUser, nil
}
func (s *workflowEventStore) SaveCheckpoint(context.Context, *naviruntime.Checkpoint) error {
	return nil
}
func (s *workflowEventStore) LoadCheckpoint(context.Context, string) (*naviruntime.Checkpoint, error) {
	return nil, nil
}
func (s *workflowEventStore) CompleteRun(context.Context, *naviruntime.RunState, string, string, string) (string, error) {
	return "", nil
}
func (s *workflowEventStore) AppendAssistantMessage(context.Context, *naviruntime.RunState, string, string, string) (string, error) {
	return "", nil
}
func (s *workflowEventStore) MarkRunCompleted(context.Context, *naviruntime.RunState, int, string) error {
	return nil
}
func (s *workflowEventStore) AppendRuntimeEvent(_ context.Context, ev schema.Event) error {
	s.events = append(s.events, ev)
	return nil
}
func (s *workflowEventStore) ListActiveRuns(context.Context) ([]*naviruntime.RunState, error) {
	return nil, nil
}

func TestProgrammerWorkflowContractPath_SelectsTicketDrivenForLinearSource(t *testing.T) {
	t.Parallel()

	if got := programmerWorkflowContractPath("linear", nil); got != programmerWorkflowTicketDrivenContractPath {
		t.Fatalf("expected linear source to use ticket-driven contract, got %q", got)
	}
	if got := programmerWorkflowContractPath("chat", nil); got != programmerWorkflowBoundedContractPath {
		t.Fatalf("expected chat source to use bounded contract, got %q", got)
	}
}

func TestProgrammerWorkflowContractPath_SelectsSelfUpdateForNormalizedSelfTarget(t *testing.T) {
	t.Parallel()

	evidence := map[string]any{
		"normalized_task": map[string]any{
			"task_class": "self_update_candidate",
		},
	}

	if got := programmerWorkflowContractPath("chat", evidence); got != programmerWorkflowSelfUpdateContractPath {
		t.Fatalf("expected self-update task to use self-update contract, got %q", got)
	}
}

func TestCaptureProgrammerWorkflowEvidence_UsesTicketDrivenContractForLinearSource(t *testing.T) {
	original := invokeProgrammerWorkflow
	defer func() { invokeProgrammerWorkflow = original }()

	var startArgs map[string]any
	invokeProgrammerWorkflow = func(_ context.Context, _ string, iface string, arguments map[string]any) (map[string]any, error) {
		switch iface {
		case "start_run":
			startArgs = arguments
			return map[string]any{
				"programmer_run": map[string]any{
					"workflow_id": "navi.programmer.ticket_driven_coding",
					"state":       "received",
				},
				"evidence": map[string]any{
					"raw_task_input": map[string]any{
						"source":    "linear",
						"source_id": "OMN-247",
						"content":   "Implement ticket ingestion.",
					},
				},
			}, nil
		case "record_step":
			return map[string]any{
				"evidence": map[string]any{
					"raw_task_input": map[string]any{
						"source":    "linear",
						"source_id": "OMN-247",
						"content":   "Implement ticket ingestion.",
					},
					"normalized_task": map[string]any{
						"summary":           "Implement ticket ingestion.",
						"task_class":        "ticket_driven_mutation",
						"acceptance_target": "Preserve source metadata.",
					},
				},
				"step_evidence": map[string]any{
					"state": "normalized",
				},
			}, nil
		case "synthesize_result":
			return map[string]any{
				"outcome":          "blocked",
				"current_state":    "blocked",
				"task_source":      "linear",
				"source_reference": map[string]any{"source": "linear", "source_id": "OMN-247"},
			}, nil
		default:
			return map[string]any{}, nil
		}
	}

	loop := &AgentLoop{}
	run := &naviruntime.RunState{
		RunID:            "run-1",
		RuntimeSessionID: "runtime-1",
		StartedAt:        time.Now().UTC(),
	}
	inbox := &naviruntime.InboxItem{
		ID:               "inbox-1",
		SourceChannel:    "linear",
		SourceMessageRef: "OMN-247",
		Content:          "Implement ticket ingestion.",
	}
	tc := llm.ToolCall{
		ID:   "tool-1",
		Name: "navi-programmer.task-normalize.normalize_task",
		Arguments: map[string]any{
			"raw_task": "Implement ticket ingestion.",
		},
	}
	tool := &navitool.Tool{
		Name:     tc.Name,
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-programmer.task-normalize",
		Metadata: navitool.ToolMetadata{SkillInterface: "normalize_task"},
	}
	rawResult := `{"status":"success","payload":{"raw_task":{"source":"linear","source_id":"OMN-247","content":"Implement ticket ingestion."},"normalized_task":{"summary":"Implement ticket ingestion.","task_class":"ticket_driven_mutation","acceptance_target":"Preserve source metadata."}}}`

	if err := loop.captureProgrammerWorkflowEvidence(context.Background(), run, inbox, tc, tool, nil, nil, rawResult); err != nil {
		t.Fatalf("captureProgrammerWorkflowEvidence returned error: %v", err)
	}
	if startArgs == nil {
		t.Fatal("expected start_run invocation")
	}
	if got := startArgs["contract_path"]; got != programmerWorkflowTicketDrivenContractPath {
		t.Fatalf("expected ticket-driven contract path, got %#v", got)
	}
	if got := startArgs["source"]; got != "linear" {
		t.Fatalf("expected source linear, got %#v", got)
	}
	if got := startArgs["source_id"]; got != "OMN-247" {
		t.Fatalf("expected source_id OMN-247, got %#v", got)
	}
	if !strings.Contains(run.Scratchpad[programmerWorkflowScratchpadKey], "ticket_driven_coding") {
		t.Fatalf("expected workflow scratchpad to preserve ticket-driven workflow, got %q", run.Scratchpad[programmerWorkflowScratchpadKey])
	}
}

func TestCaptureProgrammerWorkflowEvidence_UsesSelfUpdateContractForSelfTargetedNormalization(t *testing.T) {
	original := invokeProgrammerWorkflow
	defer func() { invokeProgrammerWorkflow = original }()

	var startArgs map[string]any
	invokeProgrammerWorkflow = func(_ context.Context, _ string, iface string, arguments map[string]any) (map[string]any, error) {
		switch iface {
		case "start_run":
			startArgs = arguments
			return map[string]any{
				"programmer_run": map[string]any{
					"workflow_id": "navi.programmer.self_update_candidate",
					"state":       "received",
				},
				"contract_path": programmerWorkflowSelfUpdateContractPath,
				"evidence": map[string]any{
					"raw_task_input": map[string]any{
						"source":  "chat",
						"content": "Improve NAVI itself safely.",
					},
				},
			}, nil
		case "record_step":
			return map[string]any{
				"workflow_id": "navi.programmer.self_update_candidate",
				"evidence": map[string]any{
					"raw_task_input": map[string]any{
						"source":  "chat",
						"content": "Improve NAVI itself safely.",
					},
					"normalized_task": map[string]any{
						"summary":     "Improve NAVI itself safely.",
						"task_class":  "self_update_candidate",
						"self_update": true,
					},
				},
				"step_evidence": map[string]any{
					"state": "normalized",
				},
			}, nil
		case "synthesize_result":
			return map[string]any{
				"workflow_id":       "navi.programmer.self_update_candidate",
				"outcome":           "blocked",
				"current_state":     "blocked",
				"verification_tier": "locally_validated_only",
			}, nil
		default:
			return map[string]any{}, nil
		}
	}

	loop := &AgentLoop{}
	run := &naviruntime.RunState{
		RunID:            "run-self-update",
		RuntimeSessionID: "runtime-self-update",
		StartedAt:        time.Now().UTC(),
	}
	inbox := &naviruntime.InboxItem{
		ID:            "inbox-self-update",
		SourceChannel: "chat",
		Content:       "Improve NAVI itself safely.",
	}
	tc := llm.ToolCall{
		ID:   "tool-self-update",
		Name: "navi-programmer.task-normalize.normalize_task",
		Arguments: map[string]any{
			"raw_task": "Improve NAVI itself safely.",
		},
	}
	tool := &navitool.Tool{
		Name:     tc.Name,
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-programmer.task-normalize",
		Metadata: navitool.ToolMetadata{SkillInterface: "normalize_task"},
	}
	rawResult := `{"status":"success","payload":{"normalized_task":{"summary":"Improve NAVI itself safely.","task_class":"self_update_candidate","self_update":true}}}`

	if err := loop.captureProgrammerWorkflowEvidence(context.Background(), run, inbox, tc, tool, nil, nil, rawResult); err != nil {
		t.Fatalf("captureProgrammerWorkflowEvidence returned error: %v", err)
	}
	if startArgs == nil {
		t.Fatal("expected start_run invocation")
	}
	if got := startArgs["contract_path"]; got != programmerWorkflowSelfUpdateContractPath {
		t.Fatalf("expected self-update contract path, got %#v", got)
	}
	if !strings.Contains(run.Scratchpad[programmerWorkflowScratchpadKey], "self_update_candidate") {
		t.Fatalf("expected workflow scratchpad to preserve self-update workflow, got %q", run.Scratchpad[programmerWorkflowScratchpadKey])
	}
}

func TestCaptureProgrammerWorkflowEvidence_EmitsPhaseLevelProgressAndPersistsStructuredResult(t *testing.T) {
	original := invokeProgrammerWorkflow
	defer func() { invokeProgrammerWorkflow = original }()

	store := &workflowEventStore{}
	invokeProgrammerWorkflow = func(_ context.Context, _ string, iface string, arguments map[string]any) (map[string]any, error) {
		switch iface {
		case "start_run":
			return map[string]any{
				"programmer_run": map[string]any{
					"workflow_id": "navi.programmer.bounded_mutation",
					"state":       "received",
				},
				"evidence": map[string]any{
					"raw_task_input": map[string]any{
						"source":  "chat",
						"content": "Update the programmer result reporting surface.",
					},
				},
			}, nil
		case "record_step":
			return map[string]any{
				"evidence": map[string]any{
					"raw_task_input": map[string]any{
						"source":  "chat",
						"content": "Update the programmer result reporting surface.",
					},
					"normalized_task": map[string]any{
						"summary":    "Update the programmer result reporting surface.",
						"task_class": "bounded_mutation",
					},
					"changed_files": []any{
						map[string]any{
							"path": "internal/navi/programmer_workflow_bridge.go",
						},
					},
					"validation_results": []any{
						map[string]any{
							"validation_kind": "test",
							"verdict":         "passed",
							"command":         []any{"go", "test", "./internal/navi"},
						},
					},
					"lifecycle_summary": map[string]any{
						"branch": "feat/programmer-reporting",
						"commit": "abc1234",
					},
				},
				"step_evidence": map[string]any{
					"state": "review_ready",
				},
			}, nil
		case "synthesize_result":
			return map[string]any{
				"outcome":       "completed",
				"current_state": "review_ready",
				"task_summary":  "Update the programmer result reporting surface.",
				"final_summary": "Changed programmer workflow reporting and verified it with go test.",
				"files_changed": []any{
					"internal/navi/programmer_workflow_bridge.go",
				},
				"validation_summary": map[string]any{
					"status": "passed",
				},
				"lifecycle_summary": map[string]any{
					"branch": "feat/programmer-reporting",
					"commit": "abc1234",
				},
			}, nil
		default:
			return map[string]any{}, nil
		}
	}

	loop := &AgentLoop{cfg: LoopConfig{RuntimeStore: store}}
	run := &naviruntime.RunState{
		RunID:            "run-2",
		RuntimeSessionID: "runtime-2",
		StartedAt:        time.Now().UTC(),
	}
	inbox := &naviruntime.InboxItem{
		ID:            "inbox-2",
		SourceChannel: "chat",
		Content:       "Update the programmer result reporting surface.",
	}
	tc := llm.ToolCall{
		ID:   "tool-2",
		Name: "navi-programmer.file-mutate.write_file",
		Arguments: map[string]any{
			"raw_task": "Update the programmer result reporting surface.",
		},
	}
	tool := &navitool.Tool{
		Name:     tc.Name,
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-programmer.file-mutate",
		Metadata: navitool.ToolMetadata{SkillInterface: "write_file"},
	}
	rawResult := `{"status":"success","payload":{"outcome":"clean","changed_files":[{"path":"internal/navi/programmer_workflow_bridge.go"}]}}`

	if err := loop.captureProgrammerWorkflowEvidence(context.Background(), run, inbox, tc, tool, nil, nil, rawResult); err != nil {
		t.Fatalf("captureProgrammerWorkflowEvidence returned error: %v", err)
	}

	result := programmerWorkflowResultFromRun(run)
	if got := anyString(mapValue(result, "lifecycle_summary", "branch")); got != "feat/programmer-reporting" {
		t.Fatalf("expected structured result branch, got %#v", got)
	}
	if got := run.Scratchpad[programmerWorkflowStateScratchpadKey]; got != "review_ready" {
		t.Fatalf("expected workflow state scratchpad to track review_ready, got %q", got)
	}

	var found bool
	for _, ev := range store.events {
		if ev.Type != schema.FactToolCallProgress {
			continue
		}
		payload, ok := ev.Payload.(schema.ToolCallProgressPayload)
		if !ok {
			t.Fatalf("expected tool progress payload, got %T", ev.Payload)
		}
		if payload.Stage == "review_ready" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected phase-level review_ready progress event, got %#v", store.events)
	}
}

func TestAttachProgrammerWorkflowResult_AddsStructuredResultToExecuteResult(t *testing.T) {
	state := programmerWorkflowState{
		CurrentState: "review_ready",
		LastResult: map[string]any{
			"outcome":      "completed",
			"task_summary": "Update the programmer result reporting surface.",
			"lifecycle_summary": map[string]any{
				"branch": "feat/programmer-reporting",
				"commit": "abc1234",
			},
		},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	run := &naviruntime.RunState{
		Scratchpad: map[string]string{
			programmerWorkflowScratchpadKey:        string(encoded),
			programmerWorkflowSummaryScratchpadKey: "state=review_ready outcome=completed",
		},
	}
	result := &naviruntime.ExecuteResult{}

	attachProgrammerWorkflowResult(result, run)

	if result.ProgrammerResult == nil {
		t.Fatal("expected structured programmer result to be attached")
	}
	if got := anyString(mapValue(result.ProgrammerResult, "lifecycle_summary", "commit")); got != "abc1234" {
		t.Fatalf("expected commit metadata in structured result, got %#v", got)
	}
	if got := result.OutcomeSummary; got != "state=review_ready outcome=completed" {
		t.Fatalf("expected outcome summary to default from workflow summary, got %q", got)
	}
}

func TestAttachProgrammerWorkflowResult_PreservesCandidateContextInStructuredResult(t *testing.T) {
	state := programmerWorkflowState{
		CurrentState: "review_ready",
		LastResult: map[string]any{
			"outcome":    "completed",
			"task_class": "self_update_candidate",
			"candidate_context": map[string]any{
				"candidate_repo_root":       ".candidate/navi",
				"trusted_runtime_untouched": true,
			},
		},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	run := &naviruntime.RunState{
		Scratchpad: map[string]string{
			programmerWorkflowScratchpadKey: string(encoded),
		},
	}
	result := &naviruntime.ExecuteResult{}

	attachProgrammerWorkflowResult(result, run)

	candidateContext, ok := result.ProgrammerResult["candidate_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected candidate_context in programmer result, got %#v", result.ProgrammerResult["candidate_context"])
	}
	if got := anyString(candidateContext["candidate_repo_root"]); got != ".candidate/navi" {
		t.Fatalf("expected candidate repo root, got %#v", got)
	}
}

func TestPrepareProgrammerToolArguments_BlocksMutativeSkillWithoutWorkspaceBinding(t *testing.T) {
	t.Parallel()

	run := &naviruntime.RunState{}
	tc := llm.ToolCall{
		Name: "navi-programmer.file-mutate.write_file",
		Arguments: map[string]any{
			"path":    "plugins/navi-programmer/README.md",
			"content": "updated",
		},
	}
	tool := &navitool.Tool{
		Name:     tc.Name,
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-programmer.file-mutate",
		Metadata: navitool.ToolMetadata{SkillInterface: "write_file"},
	}

	_, err := prepareProgrammerToolArguments(`C:\repo`, run, tc, tool, nil, nil)
	if err == nil {
		t.Fatal("expected missing workspace binding to block mutative programmer tool")
	}
	if failure := navitool.ClassifyExecutionFailure(err); failure.Code != navitool.ExecutionFailureCodeGovernanceBlocked {
		t.Fatalf("expected governance_blocked failure code, got %#v", failure)
	}
}

func TestPrepareProgrammerToolArguments_InjectsBindingAndRejectsOutOfScopePath(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(`C:\repo`)
	run := &naviruntime.RunState{}
	state := &programmerWorkflowState{
		Evidence: map[string]any{
			"workspace_binding": map[string]any{
				"repo_root":     repoRoot,
				"allowed_scope": []any{"plugins/navi-programmer"},
			},
		},
	}
	if err := persistProgrammerWorkflowState(run, state); err != nil {
		t.Fatalf("persistProgrammerWorkflowState: %v", err)
	}

	tool := &navitool.Tool{
		Name:     "navi-programmer.file-mutate.write_file",
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-programmer.file-mutate",
		Metadata: navitool.ToolMetadata{SkillInterface: "write_file"},
	}

	allowedCall := llm.ToolCall{
		Name: "navi-programmer.file-mutate.write_file",
		Arguments: map[string]any{
			"path":    "plugins/navi-programmer/README.md",
			"content": "updated",
		},
	}
	prepared, err := prepareProgrammerToolArguments(`C:\workspace`, run, allowedCall, tool, nil, nil)
	if err != nil {
		t.Fatalf("prepareProgrammerToolArguments returned error: %v", err)
	}
	if got := anyString(prepared["root"]); got != repoRoot {
		t.Fatalf("expected bound root %q, got %#v", repoRoot, got)
	}
	if got := anyString(prepared["repo_root"]); got != repoRoot {
		t.Fatalf("expected bound repo_root %q, got %#v", repoRoot, got)
	}

	blockedCall := llm.ToolCall{
		Name: "navi-programmer.file-mutate.write_file",
		Arguments: map[string]any{
			"path":    "docs/AGENTS.md",
			"content": "updated",
		},
	}
	_, err = prepareProgrammerToolArguments(`C:\workspace`, run, blockedCall, tool, nil, nil)
	if err == nil {
		t.Fatal("expected out-of-scope mutation to be blocked")
	}
	if !strings.Contains(err.Error(), "outside the bound scope") {
		t.Fatalf("expected out-of-scope error, got %v", err)
	}
}

func TestPrepareProgrammerToolArguments_SelfUpdateBindScopeDefaultsToWorkspaceRepo(t *testing.T) {
	t.Parallel()

	run := &naviruntime.RunState{}
	state := &programmerWorkflowState{
		Evidence: map[string]any{
			"normalized_task": map[string]any{
				"task_class": "self_update_candidate",
			},
		},
	}
	if err := persistProgrammerWorkflowState(run, state); err != nil {
		t.Fatalf("persistProgrammerWorkflowState: %v", err)
	}
	tc := llm.ToolCall{
		Name: "navi-programmer.task-normalize.bind_scope",
		Arguments: map[string]any{
			"normalized_task": map[string]any{
				"task_class": "self_update_candidate",
			},
		},
	}
	tool := &navitool.Tool{
		Name:     tc.Name,
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-programmer.task-normalize",
		Metadata: navitool.ToolMetadata{SkillInterface: "bind_scope"},
	}

	prepared, err := prepareProgrammerToolArguments(`C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`, run, tc, tool, nil, nil)
	if err != nil {
		t.Fatalf("prepareProgrammerToolArguments returned error: %v", err)
	}
	if got := anyString(prepared["current_repo"]); got != `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi` {
		t.Fatalf("expected self-update bind_scope to inherit current repo, got %#v", got)
	}
}
