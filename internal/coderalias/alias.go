// Package coderalias is the single source of truth for the bidirectional,
// compatibility-safe alias mapping between the legacy NAVI "Programmer" identity
// and the forward-facing NAVI "Coder" identity (migration ticket NEW-A).
//
// # Migration invariant (NEW-A)
//
// This package CREATES COMPATIBILITY; it does NOT rename the product substrate.
// The legacy identifiers — plugin id "navi.programmer", skill ids
// "navi-programmer.*", and workflow ids "navi.programmer.*" — remain canonical
// AT REST. NEW-A never edits plugin.yaml, SKILL.yaml, compiled workflow JSON,
// fixtures, or prompt files. The Coder-facing identifiers ("navi.coder",
// "navi-coder.*", "navi.coder.*") are aliases that resolve to the existing
// Programmer implementation. The physical rename / package move is OMN-283 and
// begins only after NEW-A passes tests.
//
// # Reversibility
//
// Every mapping is a bounded table lookup. Deleting this package and its call
// sites restores exact prior behavior.
//
// # Collision safety
//
// A SEPARATE, real plugin (plugins/navi-coder/, plugin id "navi-coder", skill
// id "navi.coder.repo") already exists and is unrelated to this alias. All
// functions here are table-driven (lookup-or-return-unchanged) with NO string
// surgery (no Replace/TrimPrefix+concat), so unrelated identifiers — including
// the dotted "navi.coder.repo" skill and the bare "navi-coder" plugin id —
// always pass through untouched.
//
// # Scope
//
// This package covers the plugin, skill, and workflow id classes. The prompt
// kind alias ("ncos/programmer_workflow" <-> "ncos/coder_workflow") is owned by
// the internal/prompts package, which holds the ncos/* namespace.
package coderalias

import "strings"

const (
	// LegacyPluginID is the canonical-at-rest Programmer plugin id.
	LegacyPluginID = "navi.programmer"
	// CoderPluginID is the forward-facing Coder alias for LegacyPluginID.
	// NOTE: distinct from the unrelated real plugin id "navi-coder" (hyphen).
	CoderPluginID = "navi.coder"

	legacySkillPrefix = "navi-programmer."
	coderSkillPrefix  = "navi-coder."

	legacyWorkflowPrefix = "navi.programmer."
	coderWorkflowPrefix  = "navi.coder."
)

// skillSuffixes is the bounded set of the seven Programmer skill families.
var skillSuffixes = []string{
	"file-mutate",
	"git-lifecycle",
	"patch-apply",
	"repo-inspect",
	"remote-review",
	"run-validation",
	"task-normalize",
}

// workflowSuffixes is the bounded set of the three Programmer workflow contracts.
var workflowSuffixes = []string{
	"bounded_mutation",
	"self_update_candidate",
	"ticket_driven_coding",
}

var (
	coderToLegacySkill    = map[string]string{}
	legacyToCoderSkill    = map[string]string{}
	coderToLegacyWorkflow = map[string]string{}
	legacyToCoderWorkflow = map[string]string{}
)

func init() {
	for _, s := range skillSuffixes {
		legacy := legacySkillPrefix + s
		coder := coderSkillPrefix + s
		coderToLegacySkill[coder] = legacy
		legacyToCoderSkill[legacy] = coder
	}
	for _, w := range workflowSuffixes {
		legacy := legacyWorkflowPrefix + w
		coder := coderWorkflowPrefix + w
		coderToLegacyWorkflow[coder] = legacy
		legacyToCoderWorkflow[legacy] = coder
	}
}

// CanonicalSkillID returns the legacy (canonical-at-rest) skill id for id. A
// known Coder skill alias is mapped to its Programmer equivalent; every other
// id — legacy ids, the unrelated "navi.coder.repo" skill, and any id outside the
// seven known skills — is returned unchanged.
func CanonicalSkillID(id string) string {
	if legacy, ok := coderToLegacySkill[id]; ok {
		return legacy
	}
	return id
}

// CoderSkillID returns the Coder-facing alias for a legacy skill id. Non-legacy
// ids pass through unchanged. Inverse of CanonicalSkillID over the known set.
func CoderSkillID(id string) string {
	if coder, ok := legacyToCoderSkill[id]; ok {
		return coder
	}
	return id
}

// IsProgrammerScopedSkillID reports whether id belongs to either the legacy
// Programmer skill namespace ("navi-programmer.") or the Coder skill namespace
// ("navi-coder."). It replaces the literal
// strings.HasPrefix(id, "navi-programmer.") guards while preserving their exact
// semantics for legacy ids and additionally accepting the Coder alias.
//
// Both namespaces are hyphen-scoped, so this never matches the dotted
// "navi.coder.repo" skill or the bare "navi-coder" plugin id.
func IsProgrammerScopedSkillID(id string) bool {
	return strings.HasPrefix(id, legacySkillPrefix) || strings.HasPrefix(id, coderSkillPrefix)
}

// CanonicalWorkflowID returns the legacy (canonical-at-rest) workflow id for id.
// A known Coder workflow alias is mapped to its Programmer equivalent; every
// other id is returned unchanged. Implemented as an exact-set map lookup on the
// full id (never a prefix rewrite), so the unrelated "navi.coder.repo" can never
// be mis-mapped.
func CanonicalWorkflowID(id string) string {
	if legacy, ok := coderToLegacyWorkflow[id]; ok {
		return legacy
	}
	return id
}

// CoderWorkflowID returns the Coder-facing alias for a legacy workflow id.
// Non-legacy ids pass through unchanged. Inverse of CanonicalWorkflowID.
func CoderWorkflowID(id string) string {
	if coder, ok := legacyToCoderWorkflow[id]; ok {
		return coder
	}
	return id
}

// CanonicalPluginID maps the Coder plugin alias ("navi.coder") to the legacy
// plugin id ("navi.programmer"). All other ids — including the unrelated real
// plugin id "navi-coder" (hyphen) — pass through unchanged.
func CanonicalPluginID(id string) string {
	if id == CoderPluginID {
		return LegacyPluginID
	}
	return id
}

// PluginIDsEquivalent reports whether a and b denote the same plugin under the
// Programmer<->Coder alias. The equivalence set is exactly
// {"navi.programmer", "navi.coder"}; the real hyphenated "navi-coder" plugin is
// NOT equivalent to either.
func PluginIDsEquivalent(a, b string) bool {
	return CanonicalPluginID(a) == CanonicalPluginID(b)
}
