package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/blob"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestServiceCreateArtifact_ValidatesJSONPayload(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Config",
		ArtifactType:    schema.ArtifactTypeData,
		ArtifactSubtype: "json",
		ContentFormat:   "application/json",
		Payload: map[string]any{
			"name": "navi",
			"ok":   true,
		},
		ActorType: schema.ActorAgent,
		ActorID:   "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	version, err := store.GetArtifactVersion(ctx, svc.db, artifact.CurrentVersionID)
	if err != nil {
		t.Fatalf("GetArtifactVersion: %v", err)
	}
	reader, err := svc.blob.Get(ctx, version.ContentRef.StorageKey)
	if err != nil {
		t.Fatalf("blob.Get: %v", err)
	}
	defer reader.Close()

	buf, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	got := string(buf)
	if got != "{\"name\":\"navi\",\"ok\":true}" {
		t.Fatalf("stored payload = %q", got)
	}
}

func TestServiceCreateArtifact_RejectsInvalidJSONPayload(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	_, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Broken Config",
		ArtifactType:    schema.ArtifactTypeData,
		ArtifactSubtype: "json",
		ContentFormat:   "application/json",
		Payload:         "{broken",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err == nil {
		t.Fatalf("expected invalid json error")
	}
	items, listErr := store.ListArtifactsForOwner(ctx, svc.db, "owner-1", 10)
	if listErr != nil {
		t.Fatalf("ListArtifactsForOwner: %v", listErr)
	}
	if len(items) != 0 {
		t.Fatalf("expected no artifact rows on pre-commit create failure, got %d", len(items))
	}
}

func TestServiceUpdateContent_UsesSubtypePatchStrategy(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Title\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	version, err := svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "op-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Payload:               "## Updated\n",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if version.PatchStrategy != PatchStrategyBlock {
		t.Fatalf("patch strategy = %q, want %q", version.PatchStrategy, PatchStrategyBlock)
	}
	if version.DiffRef == nil {
		t.Fatalf("expected diff ref to be recorded")
	}
	reader, err := svc.blob.Get(ctx, version.DiffRef.StorageKey)
	if err != nil {
		t.Fatalf("blob.Get diff: %v", err)
	}
	defer reader.Close()
	diffRaw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll diff: %v", err)
	}
	if string(diffRaw) == "" {
		t.Fatalf("expected diff payload")
	}
}

func TestServiceUpdateContent_AppliesMarkdownTextRangePatch(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-text-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Title\nSection\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	version, err := svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "op-text-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Payload: map[string]any{
			"mode":  "text_range",
			"start": 8,
			"end":   16,
			"text":  "Updated\n",
		},
		ActorType: schema.ActorAgent,
		ActorID:   "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	content, err := svc.ReadVersionContent(ctx, version.ID)
	if err != nil {
		t.Fatalf("ReadVersionContent: %v", err)
	}
	if content != "# Title\nUpdated\n" {
		t.Fatalf("patched content = %q", content)
	}
}

func TestServiceUpdateContent_AppliesStructuredJSONPatch(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-json-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Config",
		ArtifactType:    schema.ArtifactTypeData,
		ArtifactSubtype: "json",
		ContentFormat:   "application/json",
		Payload: map[string]any{
			"name": "navi",
			"port": 6284,
		},
		ActorType: schema.ActorAgent,
		ActorID:   "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	version, err := svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "op-json-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Payload: map[string]any{
			"mode": "structured",
			"value": map[string]any{
				"enabled": true,
			},
		},
		ActorType: schema.ActorAgent,
		ActorID:   "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	content, err := svc.ReadVersionContent(ctx, version.ID)
	if err != nil {
		t.Fatalf("ReadVersionContent: %v", err)
	}
	if content != "{\"enabled\":true,\"name\":\"navi\",\"port\":6284}" && content != "{\"name\":\"navi\",\"port\":6284,\"enabled\":true}" {
		t.Fatalf("patched json content = %q", content)
	}
}

func TestServiceUpdateContent_AppliesTableRowCellPatch(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-table-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Scores",
		ArtifactType:    schema.ArtifactTypeData,
		ArtifactSubtype: "csv",
		ContentFormat:   "text/csv",
		Payload:         "name,score\nada,1\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	version, err := svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "op-table-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Payload: map[string]any{
			"mode":   "row_cell",
			"row":    0,
			"column": "score",
			"value":  "7",
		},
		ActorType: schema.ActorAgent,
		ActorID:   "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	content, err := svc.ReadVersionContent(ctx, version.ID)
	if err != nil {
		t.Fatalf("ReadVersionContent: %v", err)
	}
	if content != "name,score\nada,7\n" {
		t.Fatalf("patched csv content = %q", content)
	}
}

func TestServiceUpdateContent_FailedPatchKeepsHeadAndRecordsFailure(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-patch-fail-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Title\nBody\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	headVersionID := artifact.CurrentVersionID

	_, err = svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "op-patch-fail-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Payload: map[string]any{
			"mode":  "text_range",
			"start": 20,
			"end":   30,
			"text":  "Nope",
		},
		ActorType: schema.ActorAgent,
		ActorID:   "navi",
	})
	if err == nil {
		t.Fatal("expected patch failure")
	}

	updated, err := store.GetArtifact(ctx, svc.db, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if updated.CurrentVersionID != headVersionID {
		t.Fatalf("expected head version to remain %s, got %s", headVersionID, updated.CurrentVersionID)
	}
	if updated.LastError == "" || updated.RecoverableOutput == "" {
		t.Fatalf("expected recorded failure metadata, got last_error=%q recoverable_output=%q", updated.LastError, updated.RecoverableOutput)
	}
	if got := updated.Attributes["failure_class"]; got != string(schema.FailureClassPatchApplyFailed) {
		t.Fatalf("expected failure_class %q, got %v", schema.FailureClassPatchApplyFailed, got)
	}
}

func TestServiceCreateArtifact_PersistsConversationAndProjectReferences(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		OperationID:          "op-1",
		WorkspaceID:          "ws-1",
		ProjectID:            "proj-1",
		OwnerID:              "owner-1",
		Title:                "Doc",
		ArtifactType:         schema.ArtifactTypeDocument,
		ArtifactSubtype:      "markdown",
		ContentFormat:        "text/markdown",
		Payload:              "# Title\n",
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: "sess-1",
		SourceMessageID:      "msg-1",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	conversationRefs, err := store.ListArtifactIDsBySource(ctx, svc.db, "conversation", "sess-1", 10)
	if err != nil {
		t.Fatalf("ListArtifactIDsBySource conversation: %v", err)
	}
	if len(conversationRefs) != 1 || conversationRefs[0] != artifact.ID {
		t.Fatalf("unexpected conversation refs: %v", conversationRefs)
	}

	projectRefs, err := store.ListArtifactIDsBySource(ctx, svc.db, "project", "proj-1", 10)
	if err != nil {
		t.Fatalf("ListArtifactIDsBySource project: %v", err)
	}
	if len(projectRefs) != 1 || projectRefs[0] != artifact.ID {
		t.Fatalf("unexpected project refs: %v", projectRefs)
	}
}

func TestServiceExportArtifact_CompletesDownloadExport(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-export-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Export me\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	exported, err := svc.ExportArtifact(ctx, ExportRequest{
		ArtifactID: artifact.ID,
		Format:     schema.ExportFormatPDF,
		TargetKind: schema.ArtifactExportTargetDownload,
		ActorType:  schema.ActorAgent,
		ActorID:    "navi",
	})
	if err != nil {
		t.Fatalf("ExportArtifact: %v", err)
	}
	if exported.Status != schema.ArtifactExportCompleted || exported.ContentRef == nil {
		t.Fatalf("unexpected export record: %+v", exported)
	}
	payload, item, err := svc.ReadExportContent(ctx, exported.ExportID)
	if err != nil {
		t.Fatalf("ReadExportContent: %v", err)
	}
	if item.ContentType != "application/pdf" || !bytes.HasPrefix(payload, []byte("%PDF-1.4")) {
		t.Fatalf("unexpected export payload/content type: %s %q", item.ContentType, string(payload))
	}

	exports, err := store.ListArtifactExports(ctx, svc.db, artifact.ID, 10)
	if err != nil {
		t.Fatalf("ListArtifactExports: %v", err)
	}
	if len(exports) != 1 {
		t.Fatalf("expected 1 export record, got %d", len(exports))
	}
}

func TestServiceExportArtifact_RecordsFailedGovernanceCheck(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-export-2",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Export me\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	_, err = svc.ExportArtifact(ctx, ExportRequest{
		ArtifactID: artifact.ID,
		Format:     schema.ExportFormatMarkdown,
		TargetKind: schema.ArtifactExportTargetExternalSystem,
		ActorType:  schema.ActorAgent,
		ActorID:    "navi",
	})
	if err == nil {
		t.Fatal("expected governance failure")
	}
	exports, err := store.ListArtifactExports(ctx, svc.db, artifact.ID, 10)
	if err != nil {
		t.Fatalf("ListArtifactExports: %v", err)
	}
	if len(exports) != 1 || exports[0].Status != schema.ArtifactExportFailed || exports[0].FailureClass != schema.FailureClassPolicyBlocked {
		t.Fatalf("unexpected failed export record: %+v", exports)
	}
}

func TestServiceExportArtifact_FailureLeavesContentUnchangedAndPreservesInProgressState(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-export-3",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Draft Export",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Keep me\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	artifact.LifecycleState = schema.ArtifactLifecycleInProgress
	if err := store.SaveArtifact(ctx, svc.db, artifact); err != nil {
		t.Fatalf("SaveArtifact: %v", err)
	}

	headVersionID := artifact.CurrentVersionID
	headContent, err := svc.ReadVersionContent(ctx, headVersionID)
	if err != nil {
		t.Fatalf("ReadVersionContent before export: %v", err)
	}

	_, err = svc.ExportArtifact(ctx, ExportRequest{
		ArtifactID: artifact.ID,
		Format:     schema.ExportFormatMarkdown,
		TargetKind: schema.ArtifactExportTargetExternalSystem,
		ActorType:  schema.ActorAgent,
		ActorID:    "navi",
	})
	if err == nil {
		t.Fatal("expected governance failure")
	}

	updated, err := store.GetArtifact(ctx, svc.db, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if updated.CurrentVersionID != headVersionID {
		t.Fatalf("expected head version to remain %s, got %s", headVersionID, updated.CurrentVersionID)
	}
	if updated.LifecycleState != schema.ArtifactLifecycleInProgress {
		t.Fatalf("expected lifecycle to remain in_progress, got %s", updated.LifecycleState)
	}
	if updated.LastError == "" || updated.RecoverableOutput == "" {
		t.Fatalf("expected recovery metadata, got last_error=%q recoverable_output=%q", updated.LastError, updated.RecoverableOutput)
	}
	if got := updated.Attributes["failure_class"]; got != string(schema.FailureClassPolicyBlocked) {
		t.Fatalf("expected failure_class %q, got %v", schema.FailureClassPolicyBlocked, got)
	}

	stillHead, err := svc.ReadVersionContent(ctx, updated.CurrentVersionID)
	if err != nil {
		t.Fatalf("ReadVersionContent after export: %v", err)
	}
	if stillHead != headContent {
		t.Fatalf("expected content to remain unchanged, got %q", stillHead)
	}
}

func TestServiceShareArtifact_PersistsLinkShare(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-share-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Share me\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	shared, err := svc.ShareArtifact(ctx, ShareRequest{
		ArtifactID:       artifact.ID,
		Scope:            schema.ArtifactShareScopeLink,
		AccessLevel:      schema.ArtifactShareAccessRead,
		ActorType:        schema.ActorAgent,
		ActorID:          "navi",
		ConfirmationMode: "proposal",
	})
	if err != nil {
		t.Fatalf("ShareArtifact: %v", err)
	}
	if shared.Status != schema.ArtifactShareStatusActive || shared.ShareURL == "" {
		t.Fatalf("unexpected share record: %+v", shared)
	}
	shares, err := store.ListArtifactShares(ctx, svc.db, artifact.ID, 10)
	if err != nil {
		t.Fatalf("ListArtifactShares: %v", err)
	}
	if len(shares) != 1 || shares[0].ShareID != shared.ShareID {
		t.Fatalf("unexpected shares: %+v", shares)
	}
}

func TestServiceBranchArtifact_CreatesAndSwitchesBranch(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-branch-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Branch me\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	branch, err := svc.BranchArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpBranch,
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Reason:                "conflict-fallback",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err != nil {
		t.Fatalf("BranchArtifact: %v", err)
	}
	if branch.Name != "conflict-fallback" || branch.HeadVersionID != artifact.CurrentVersionID {
		t.Fatalf("unexpected branch: %+v", branch)
	}
	updated, err := store.GetArtifact(ctx, svc.db, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if updated.CurrentBranchID != branch.ID {
		t.Fatalf("expected artifact current branch %s, got %s", branch.ID, updated.CurrentBranchID)
	}
}

func TestServiceRestoreVersion_CreatesNewHeadFromHistoricalVersion(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	var events []schema.Event
	svc.SetEventPublisher(func(ctx context.Context, ev schema.Event) error {
		events = append(events, ev)
		return nil
	})

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-restore-1",
		WorkspaceID:     "ws-1",
		ProjectID:       "proj-1",
		OwnerID:         "owner-1",
		Title:           "Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# First\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	originalVersionID := artifact.CurrentVersionID
	updated, err := svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpReplace,
		OperationID:           "op-restore-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: artifact.CurrentVersionID,
		Payload:               "# Second\n",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	events = nil

	restored, err := svc.RestoreVersion(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpRestoreVersion,
		OperationID:           "op-restore-3",
		TargetArtifactID:      artifact.ID,
		TargetVersionID:       originalVersionID,
		ExpectedBaseVersionID: updated.ID,
		Reason:                "Restore original",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err != nil {
		t.Fatalf("RestoreVersion: %v", err)
	}
	if restored.ID == originalVersionID || restored.ID == updated.ID {
		t.Fatalf("expected a new restored version id, got %s", restored.ID)
	}
	if restored.BaseVersionID != originalVersionID {
		t.Fatalf("expected restore base version %s, got %s", originalVersionID, restored.BaseVersionID)
	}

	content, err := svc.ReadVersionContent(ctx, restored.ID)
	if err != nil {
		t.Fatalf("ReadVersionContent: %v", err)
	}
	if content != "# First\n" {
		t.Fatalf("restored content = %q", content)
	}
	current, err := store.GetArtifact(ctx, svc.db, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if current.CurrentVersionID != restored.ID {
		t.Fatalf("expected artifact head %s, got %s", restored.ID, current.CurrentVersionID)
	}

	seen := map[schema.EventType]bool{}
	for _, ev := range events {
		seen[ev.Type] = true
	}
	for _, want := range []schema.EventType{
		schema.FactArtifactExecutionStarted,
		schema.FactArtifactVersionRestoreRequested,
		schema.FactArtifactRendererResolved,
		schema.FactArtifactVersionCommitted,
		schema.FactArtifactExecutionCompleted,
	} {
		if !seen[want] {
			t.Fatalf("expected event %s in %+v", want, events)
		}
	}
}

func TestServiceCreateArtifact_PostRowFailureMarksErroredArtifactWithRecoveryDraft(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	if _, err := svc.db.ExecContext(ctx, `DROP TABLE artifact_branches`); err != nil {
		t.Fatalf("DROP TABLE artifact_branches: %v", err)
	}

	_, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-create-fail-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Broken Create",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Draft\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err == nil {
		t.Fatal("expected create failure")
	}

	items, err := store.ListArtifactsForOwner(ctx, svc.db, "owner-1", 10)
	if err != nil {
		t.Fatalf("ListArtifactsForOwner: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 errored artifact, got %d", len(items))
	}
	item := items[0]
	if item.LifecycleState != schema.ArtifactLifecycleErrored {
		t.Fatalf("expected errored lifecycle state, got %s", item.LifecycleState)
	}
	if item.LastError == "" || item.RecoverableOutput == "" {
		t.Fatalf("expected recovery metadata, got last_error=%q recoverable_output=%q", item.LastError, item.RecoverableOutput)
	}
}

func TestServiceCreateArtifact_EmitsObservabilityAndReflectionHooks(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	var events []schema.Event
	var reflections []schema.ReflectionPayload
	svc.SetEventPublisher(func(ctx context.Context, ev schema.Event) error {
		events = append(events, ev)
		return nil
	})
	svc.SetReflectionPublisher(func(ctx context.Context, payload schema.ReflectionPayload) error {
		reflections = append(reflections, payload)
		return nil
	})

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		OperationID:          "op-observe-1",
		WorkspaceID:          "ws-1",
		ProjectID:            "proj-1",
		OwnerID:              "owner-1",
		Title:                "Observed Doc",
		ArtifactType:         schema.ArtifactTypeDocument,
		ArtifactSubtype:      "markdown",
		ContentFormat:        "text/markdown",
		Payload:              "# Observe\n",
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	seen := map[schema.EventType]bool{}
	for _, ev := range events {
		seen[ev.Type] = true
	}
	for _, want := range []schema.EventType{
		schema.FactArtifactExecutionStarted,
		schema.FactArtifactRendererResolved,
		schema.FactArtifactCreated,
		schema.FactArtifactVersionCommitted,
		schema.FactArtifactExecutionCompleted,
	} {
		if !seen[want] {
			t.Fatalf("expected event %s in %+v", want, events)
		}
	}
	if len(reflections) != 1 {
		t.Fatalf("expected 1 reflection payload, got %d", len(reflections))
	}
	if reflections[0].RuntimeSessionID != "sess-1" {
		t.Fatalf("expected reflection session sess-1, got %q", reflections[0].RuntimeSessionID)
	}
	if reflections[0].Details == "" || artifact.ID == "" {
		t.Fatalf("expected reflection details and artifact id, got details=%q artifact=%q", reflections[0].Details, artifact.ID)
	}
}

func TestServiceUpdateContent_ConflictEmitsObservabilityEvents(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	var events []schema.Event
	svc.SetEventPublisher(func(ctx context.Context, ev schema.Event) error {
		events = append(events, ev)
		return nil
	})

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-conflict-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Conflict Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Base\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	events = nil

	_, err = svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpReplace,
		OperationID:           "op-conflict-2",
		TargetArtifactID:      artifact.ID,
		ExpectedBaseVersionID: "stale-version",
		Payload:               "# Updated\n",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err == nil {
		t.Fatal("expected version conflict")
	}

	seen := map[schema.EventType]bool{}
	for _, ev := range events {
		seen[ev.Type] = true
	}
	if !seen[schema.FactArtifactConflictDetected] {
		t.Fatalf("expected conflict event, got %+v", events)
	}
	if !seen[schema.FactArtifactExecutionFailed] {
		t.Fatalf("expected execution failed event, got %+v", events)
	}
}

func TestServiceUpdateContent_ConcurrentWritersRejectStaleCommit(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-race-1",
		WorkspaceID:     "ws-1",
		OwnerID:         "owner-1",
		Title:           "Race Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Base\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	baseVersionID := artifact.CurrentVersionID
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)

	for i, payload := range []string{"# Update A\n", "# Update B\n"} {
		wg.Add(1)
		go func(i int, payload string) {
			defer wg.Done()
			<-start
			operationID := "op-race-2"
			if i == 1 {
				operationID = "op-race-3"
			}
			_, err := svc.UpdateContent(ctx, schema.ArtifactOperationEnvelope{
				Operation:             schema.ArtifactOpReplace,
				OperationID:           operationID,
				TargetArtifactID:      artifact.ID,
				ExpectedBaseVersionID: baseVersionID,
				Payload:               payload,
				ActorType:             schema.ActorAgent,
				ActorID:               "navi",
			})
			results <- err
		}(i, payload)
	}
	close(start)
	wg.Wait()
	close(results)

	var successCount, conflictCount int
	for err := range results {
		switch {
		case err == nil:
			successCount++
		case strings.Contains(err.Error(), "version_conflict"):
			conflictCount++
		default:
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	if successCount != 1 || conflictCount != 1 {
		t.Fatalf("expected 1 success and 1 conflict, got success=%d conflict=%d", successCount, conflictCount)
	}

	versions, err := store.ListArtifactVersions(ctx, svc.db, artifact.ID, 10)
	if err != nil {
		t.Fatalf("ListArtifactVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected exactly 2 persisted versions (base + winner), got %d", len(versions))
	}
}

func TestServiceExportArtifact_RendererMissingEmitsReflectionAndEvent(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t)
	defer cleanup()

	var events []schema.Event
	var reflections []schema.ReflectionPayload
	svc.SetEventPublisher(func(ctx context.Context, ev schema.Event) error {
		events = append(events, ev)
		return nil
	})
	svc.SetReflectionPublisher(func(ctx context.Context, payload schema.ReflectionPayload) error {
		reflections = append(reflections, payload)
		return nil
	})

	artifact, err := svc.CreateArtifact(ctx, schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "op-export-unsupported-1",
		WorkspaceID:     "ws-1",
		ProjectID:       "proj-1",
		OwnerID:         "owner-1",
		Title:           "Markdown Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Doc\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	events = nil
	reflections = nil

	if _, err := svc.ExportArtifact(ctx, ExportRequest{
		ArtifactID: artifact.ID,
		Format:     schema.ExportFormatJSON,
		TargetKind: schema.ArtifactExportTargetDownload,
		ActorType:  schema.ActorAgent,
		ActorID:    "navi",
	}); err == nil {
		t.Fatal("expected unsupported export to fail")
	}

	seen := map[schema.EventType]bool{}
	for _, ev := range events {
		seen[ev.Type] = true
	}
	if !seen[schema.FactArtifactRendererMissing] {
		t.Fatalf("expected renderer missing event, got %+v", events)
	}
	if !seen[schema.FactArtifactExecutionFailed] {
		t.Fatalf("expected execution failed event, got %+v", events)
	}
	if len(reflections) == 0 {
		t.Fatal("expected reflection payload for failed export")
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(reflections[len(reflections)-1].Details), &details); err != nil {
		t.Fatalf("unmarshal reflection details: %v", err)
	}
	if details["failure_class"] != string(schema.FailureClassRendererMissing) {
		t.Fatalf("expected failure_class renderer_missing, got %v", details["failure_class"])
	}
	if details["result_status"] != "failed" {
		t.Fatalf("expected result_status failed, got %v", details["result_status"])
	}
}

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()

	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	root := t.TempDir()
	blobStore, err := blob.NewFilesystemStore(filepath.Join(root, "blob"))
	if err != nil {
		t.Fatalf("blob.NewFilesystemStore: %v", err)
	}

	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-1",
		Name:           "Artifact Test Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/artifact-tests"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace(ws-1): %v", err)
	}
	if err := store.SaveProject(ctx, db, schema.Project{
		ID:          "proj-1",
		Title:       "Artifact Test Project",
		Kind:        schema.ProjectKindCoding,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: "ws-1",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "owner-1",
	}); err != nil {
		t.Fatalf("SaveProject(proj-1): %v", err)
	}

	svc := NewService(db, blobStore)
	return svc, func() {
		_ = db.Close()
		_ = os.RemoveAll(root)
	}
}
