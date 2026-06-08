package governor_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/schema"
)

// TestGovernedPythonRoundTrip proves the generated stdlib-only Python contracts in
// schema/python/navi_schema/governed.py serialize Go→JSON→Python and back with no
// field loss, for the two load-bearing governed types: schema.ExecutionOutcome (the
// execution ledger record) and governor.ValidationOutcome (carried inside
// ValidationResult, plus exercised bare).
//
// The test is a real cross-language round-trip: Go emits a fully-populated fixture,
// a Python child process reconstructs it through the generated dataclass and
// re-serializes it, and Go asserts the two JSON documents are structurally equal.
// It skips (rather than fails) when python or the generated module is unavailable,
// so a Go-only environment stays green while a fully-provisioned dev/CI box gets the
// real proof. Run `make generate-python` first to materialize the module.
func TestGovernedPythonRoundTrip(t *testing.T) {
	py := findPython()
	if py == "" {
		t.Skip("python/python3 not found on PATH; skipping cross-language round-trip")
	}
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "schema", "python", "navi_schema")
	if _, err := os.Stat(filepath.Join(pkgDir, "governed.py")); err != nil {
		t.Skip("schema/python/navi_schema/governed.py not generated; run `make generate-python`")
	}

	end := time.Date(2026, 6, 2, 13, 30, 0, 0, time.UTC)
	eo := schema.ExecutionOutcome{
		AttemptID:            "att-1",
		CommandID:            "cmd-1",
		AttemptNumber:        2,
		RetryOf:              "att-0",
		CommandType:          schema.CommandTypeCreate,
		StartTime:            time.Date(2026, 6, 2, 13, 0, 0, 0, time.UTC),
		EndTime:              &end,
		Outcome:              schema.ExecutionOutcomePartiallySucceeded,
		FailureClass:         schema.FailureClassPartialExecution,
		FailureReason:        "partial write",
		AffectedEntities:     "contact:42",
		Retryable:            true,
		CompensationRequired: true,
		CompensationStatus:   schema.CompensationStatusPending,
		RecoveryStatus:       schema.RecoveryStatusOpen,
		ProposalID:           "prop-1",
		RunID:                "run-1",
		RuntimeSessionID:     "rs-1",
		CorrelationID:        "corr-1",
		ParentRunID:          "run-0",
		SkillIDs:             []string{"skill.a", "skill.b"},
		ConnectorIDs:         []string{"telegram"},
		LLMProvider:          "anthropic",
		LLMModel:             "claude-opus-4-8",
		LLMTaskClass:         "coding",
		LLMComplexity:        "high",
		WorkspaceID:          "ws-1",
		BoundaryCrossing:     true,
		ApprovalRequired:     true,
		ApprovalOutcome:      schema.ApprovalOutcomeAllowOnce,
		ArtifactID:           "art-1",
	}
	assertRoundTrip(t, py, pkgDir, "ExecutionOutcome", eo)

	vr := governor.ValidationResult{
		Outcome:        governor.ValidationModified,
		Reason:         "softened risky merge",
		ModifiedAction: `{"kind":2}`,
		Tier:           governor.GovernanceTierOwner,
	}
	assertRoundTrip(t, py, pkgDir, "ValidationResult", vr)

	// ValidationOutcome on its own — the int enum must survive the trip too.
	assertRoundTrip(t, py, pkgDir, "ValidationOutcome", governor.ValidationRejected)
}

// assertRoundTrip marshals value to JSON, has Python reconstruct it through the
// generated governed type and re-serialize, then asserts structural JSON equality.
func assertRoundTrip(t *testing.T, py, pkgDir, typeName string, value any) {
	t.Helper()
	goJSON, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s fixture: %v", typeName, err)
	}

	cmd := exec.Command(py, "-c", pyRoundTripScript, pkgDir, typeName)
	cmd.Stdin = bytes.NewReader(goJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python round-trip for %s failed: %v\nstderr: %s", typeName, err, stderr.String())
	}

	var before, after any
	if err := json.Unmarshal(goJSON, &before); err != nil {
		t.Fatalf("unmarshal go json for %s: %v", typeName, err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &after); err != nil {
		t.Fatalf("unmarshal python json for %s: %v\nraw: %s", typeName, err, stdout.String())
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("round-trip mismatch for %s:\n go:     %s\n python: %s", typeName, goJSON, stdout.String())
	}
}

// pyRoundTripScript imports the generated governed module directly (bypassing the
// pydantic-dependent navi_schema __init__), reconstructs the value from stdin JSON,
// and re-serializes it to stdout. Handles both dataclasses and bare enums.
const pyRoundTripScript = `
import sys, json, dataclasses
sys.path.insert(0, sys.argv[1])
import governed
cls = getattr(governed, sys.argv[2])
data = json.load(sys.stdin)
if dataclasses.is_dataclass(cls):
    obj = cls(**data)
    print(json.dumps(dataclasses.asdict(obj)))
else:
    obj = cls(data)
    print(json.dumps(obj.value))
`

func findPython() string {
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate module root (go.mod) from %s", dir)
		}
		dir = parent
	}
}
