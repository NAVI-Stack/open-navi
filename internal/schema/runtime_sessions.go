// Package schema — RuntimeSession identity heuristics.
//
// RuntimeSessionKind in this package is a DB/ID-heuristic type used by the
// store layer to classify a runtime session from its ID or DB row when no
// richer declaration is available. It is DISTINCT from
// internal/navi.RuntimeSessionKind, which is the fuller runtime domain enum
// (user/internal/background/connector/dreaming). The two enums are kept
// intentionally separate; unification is out of scope for the current
// legacy-Session deletion tranche.
package schema

import "strings"

type RuntimeSessionKind string

const (
	RuntimeSessionKindUser     RuntimeSessionKind = "user"
	RuntimeSessionKindInternal RuntimeSessionKind = "internal"
)

// HeartbeatAutoRuntimeSessionID is the canonical internal heartbeat
// runtime-session id. The string literal is preserved so existing DB rows
// keyed on this value continue to resolve correctly after the rename.
const HeartbeatAutoRuntimeSessionID = "heartbeat-auto"

func NormalizeRuntimeSessionKind(kind string) RuntimeSessionKind {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(RuntimeSessionKindInternal):
		return RuntimeSessionKindInternal
	default:
		return RuntimeSessionKindUser
	}
}

func DefaultRuntimeSessionKindForID(runtimeSessionID string) RuntimeSessionKind {
	switch strings.ToLower(strings.TrimSpace(runtimeSessionID)) {
	case HeartbeatAutoRuntimeSessionID:
		return RuntimeSessionKindInternal
	default:
		return RuntimeSessionKindUser
	}
}

func ResolveRuntimeSessionKind(kind string, runtimeSessionID string) RuntimeSessionKind {
	if strings.TrimSpace(kind) == "" {
		return DefaultRuntimeSessionKindForID(runtimeSessionID)
	}
	return NormalizeRuntimeSessionKind(kind)
}

func IsInternalRuntimeSession(kind RuntimeSessionKind, runtimeSessionID string) bool {
	return ResolveRuntimeSessionKind(string(kind), runtimeSessionID) == RuntimeSessionKindInternal
}

func IsInternalRuntimeSessionID(runtimeSessionID string) bool {
	return IsInternalRuntimeSession("", runtimeSessionID)
}
