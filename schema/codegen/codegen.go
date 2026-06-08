// Package codegen is the language-neutral core of NAVI's governed-type codegen.
//
// It establishes the single source-of-truth input set — the seed enums and
// structs from internal/schema, internal/governor, and internal/connectors — and
// the reflection/AST machinery that turns them into a renderer-neutral Model
// (enums + topo-ordered structs). The Python emitter (schema/python/gen) and the
// TypeScript emitter (schema/ts/gen) both call Build and render that one Model, so
// both languages emit the same governed contracts from the same Go types and can
// never drift apart (Language-Layer Contract §7: generated, not hand-written).
//
// Enum *values* are read from Go source via go/ast and struct *shapes* via
// reflection, so the emitted modules never drift from the kernel.
package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/schema"
)

// PkgDirs maps a Go import path to its source directory (relative to the module
// root) for the go/ast enum-value pass.
var PkgDirs = map[string]string{
	"github.com/open-navi/navi/internal/schema":     "internal/schema",
	"github.com/open-navi/navi/internal/governor":   "internal/governor",
	"github.com/open-navi/navi/internal/connectors": "internal/connectors",
}

// EnumMember is one member of a generated enum. Name is the canonical
// SCREAMING_SNAKE identifier shared across renderers; StrVal/IntVal carry the
// underlying value (only one is meaningful, per EnumInfo.IsString).
type EnumMember struct {
	Name   string
	StrVal string
	IntVal int64
}

// EnumInfo describes one governed enum sourced from Go const declarations.
type EnumInfo struct {
	Name     string // == Go type name
	IsString bool
	Members  []EnumMember
}

// Field is one struct field, keyed by its JSON name with the original Go type so
// each renderer can map it to its own annotation. OmitEmpty mirrors the json
// tag's omitempty option — such fields may be absent from the wire payload, which
// renderers that validate (TypeScript/Zod) map to optional.
type Field struct {
	JSONName  string
	Type      reflect.Type
	OmitEmpty bool
}

// Struct describes one governed struct sourced from Go via reflection.
type Struct struct {
	Name   string // == Go type name
	GoRef  string // e.g. "schema.ExecutionOutcome" — for docstrings
	Fields []Field
	Deps   []reflect.Type // value-struct dependencies (Python class-definition order)
	Refs   []reflect.Type // all referenced kernel structs (TS const-init order)
}

// Model is the renderer-neutral contract IR: referenced enums (sorted by name)
// and structs (topo-ordered so value-struct deps precede dependents).
type Model struct {
	Enums   []*EnumInfo
	Structs []*Struct

	allEnums map[string]*EnumInfo // keyed by pkgPath.TypeName — full registry
}

// Enum resolves the EnumInfo for a Go type, if that type is a generated enum.
func (m *Model) Enum(t reflect.Type) (*EnumInfo, bool) {
	if key := TypeKey(t); key != "" {
		e, ok := m.allEnums[key]
		return e, ok
	}
	return nil, false
}

// EnumSeeds is the standalone governed enums to emit even when no struct
// references them. Tied to the Go types directly so a rename breaks the build
// rather than drifting silently.
func EnumSeeds() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(schema.CommandType("")),
		reflect.TypeOf(schema.IdempotencyExpectation("")),
		reflect.TypeOf(schema.FailureClass("")),
		reflect.TypeOf(schema.ReversibilityClass("")),
		reflect.TypeOf(schema.ComposeFailureMode("")),
		reflect.TypeOf(schema.DegradationVisibility("")),
		reflect.TypeOf(schema.ExecutionOutcomeOutcome("")),
		reflect.TypeOf(schema.CompensationStatus("")),
		reflect.TypeOf(schema.RecoveryStatus("")),
		reflect.TypeOf(schema.ApprovalOutcome("")),
		reflect.TypeOf(schema.ProposalPriority("")),
		reflect.TypeOf(schema.ProposalStatus("")),
		reflect.TypeOf(schema.ResolutionType("")),
		reflect.TypeOf(schema.RunStatus("")),
		reflect.TypeOf(schema.EventKind("")),
		reflect.TypeOf(schema.EventType("")),
		reflect.TypeOf(schema.EventVisibility("")),
		reflect.TypeOf(schema.AssistantMessageKind("")),
		reflect.TypeOf(schema.ContentTrust("")),
		reflect.TypeOf(schema.PrivacyClass("")),
		reflect.TypeOf(schema.JobMode("")),
		reflect.TypeOf(schema.AgentType("")),
		reflect.TypeOf(governor.ValidationOutcome(0)),
		reflect.TypeOf(governor.GovernanceTier("")),
		reflect.TypeOf(governor.MutationKind(0)),
	}
}

// StructSeeds is the governed structs to emit. Reflection discovers their
// transitive struct deps within the kernel packages automatically.
func StructSeeds() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(schema.ExecutionOutcome{}),
		reflect.TypeOf(schema.Proposal{}),
		reflect.TypeOf(schema.Event{}),
		reflect.TypeOf(governor.ValidationResult{}),
		reflect.TypeOf(governor.ActionDescriptor{}),
		reflect.TypeOf(governor.MutationDescriptor{}),
		reflect.TypeOf(governor.SoftenedMutation{}),
		reflect.TypeOf(connectors.CapabilityDescriptor{}),
		reflect.TypeOf(connectors.ResultEnvelope{}),
		reflect.TypeOf(connectors.EventEnvelope{}),
		// Run lifecycle DTOs.
		reflect.TypeOf(schema.RunStartedPayload{}),
		reflect.TypeOf(schema.RunPhaseChangedPayload{}),
		reflect.TypeOf(schema.InferenceDecisionTracePayload{}),
		reflect.TypeOf(schema.RunPausedPayload{}),
		reflect.TypeOf(schema.RunResumedPayload{}),
		reflect.TypeOf(schema.RunCompletedPayload{}),
		reflect.TypeOf(schema.RunFailedPayload{}),
		reflect.TypeOf(schema.RunCancelledPayload{}),
		reflect.TypeOf(schema.InterruptRaisedPayload{}),
		reflect.TypeOf(schema.InterruptAppliedPayload{}),
		// Tool-call telemetry DTOs.
		reflect.TypeOf(schema.ToolCallStartedPayload{}),
		reflect.TypeOf(schema.ToolCallProgressPayload{}),
		reflect.TypeOf(schema.ToolCallCompletedPayload{}),
		reflect.TypeOf(schema.ToolCallFailedPayload{}),
		// Governed-read audit DTO (query_context, Language-Layer Contract §4).
		reflect.TypeOf(schema.ContextReadAuditPayload{}),
		// Message-intake telemetry DTOs.
		reflect.TypeOf(schema.MessageReceivedPayload{}),
		reflect.TypeOf(schema.MessageClassifiedPayload{}),
		reflect.TypeOf(schema.MessageQueueActionPayload{}),
		// Assistant streaming telemetry DTOs.
		reflect.TypeOf(schema.AssistantTokenDeltaPayload{}),
		reflect.TypeOf(schema.AssistantMessagePartialPayload{}),
		reflect.TypeOf(schema.AssistantMessageCompletedPayload{}),
		// Governance / proposal / degradation / recovery telemetry DTOs.
		reflect.TypeOf(schema.GovernanceBlockedPayload{}),
		reflect.TypeOf(schema.ProposalWaitingPayload{}),
		reflect.TypeOf(schema.ProposalResolvedPayload{}),
		reflect.TypeOf(schema.DegradationNotedPayload{}),
		reflect.TypeOf(schema.RecoveryRequiredPayload{}),
	}
}

type builder struct {
	root     string
	enumReg  map[string]*EnumInfo
	enumRefs map[string]bool
	structs  map[reflect.Type]*Struct
	queue    []reflect.Type
}

// Build produces the contract Model from the module rooted at root. It is the one
// entry point both language emitters call.
func Build(root string) (*Model, error) {
	reg, err := extractEnums(root)
	if err != nil {
		return nil, fmt.Errorf("extracting enums: %w", err)
	}

	b := &builder{
		root:     root,
		enumReg:  reg,
		enumRefs: map[string]bool{},
		structs:  map[reflect.Type]*Struct{},
	}

	for _, t := range EnumSeeds() {
		key := TypeKey(t)
		if _, ok := reg[key]; !ok {
			return nil, fmt.Errorf("required enum %s has no constants in source", key)
		}
		b.enumRefs[key] = true
	}

	b.queue = append(b.queue, StructSeeds()...)
	b.walk()

	// Enums, sorted by name for deterministic output.
	enums := make([]*EnumInfo, 0, len(b.enumRefs))
	for k := range b.enumRefs {
		enums = append(enums, reg[k])
	}
	sort.Slice(enums, func(i, j int) bool { return enums[i].Name < enums[j].Name })

	structs := topoSortStructs(b.structs)

	// Guard against name collisions between an enum and a struct.
	seen := map[string]string{}
	for _, e := range enums {
		seen[e.Name] = "enum"
	}
	for _, s := range structs {
		if prev, ok := seen[s.Name]; ok {
			return nil, fmt.Errorf("type name collision %q (struct vs %s)", s.Name, prev)
		}
		seen[s.Name] = "struct"
	}

	return &Model{Enums: enums, Structs: structs, allEnums: reg}, nil
}

// walk processes the struct queue, building Structs and discovering transitive
// struct/enum references.
func (b *builder) walk() {
	for len(b.queue) > 0 {
		t := b.queue[0]
		b.queue = b.queue[1:]
		if _, ok := b.structs[t]; ok {
			continue
		}
		sd := &Struct{Name: t.Name(), GoRef: GoRef(t)}
		b.structs[t] = sd // register before recursing to tolerate cycles
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if f.Anonymous {
				fmt.Fprintf(os.Stderr, "codegen: warning: skipping embedded field %s in %s\n", f.Name, t.Name())
				continue
			}
			jsonKey, skip := JSONName(f)
			if skip {
				continue
			}
			var refs []reflect.Type
			b.collect(f.Type, &refs)
			sd.Refs = appendUnique(sd.Refs, refs...)
			if dep := topLevelValueDep(f.Type); dep != nil {
				sd.Deps = append(sd.Deps, dep)
			}
			sd.Fields = append(sd.Fields, Field{JSONName: jsonKey, Type: f.Type, OmitEmpty: IsOmitEmpty(f)})
		}
	}
}

// collect records any enum reference, enqueues every discovered kernel struct,
// and appends each referenced kernel struct to refs. It mirrors the discovery
// traversal the renderers' type-mapping performs, kept in one place so both
// languages discover an identical type graph.
func (b *builder) collect(t reflect.Type, refs *[]reflect.Type) {
	if t.Kind() == reflect.Pointer {
		b.collect(t.Elem(), refs)
		return
	}
	if key := TypeKey(t); key != "" {
		if _, ok := b.enumReg[key]; ok {
			b.enumRefs[key] = true
			return
		}
	}
	switch t.Kind() {
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 { // []byte / json.RawMessage
			return
		}
		b.collect(t.Elem(), refs)
	case reflect.Map:
		b.collect(t.Key(), refs)
		b.collect(t.Elem(), refs)
	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return // RFC3339 timestamp
		}
		if IsKernelPkg(t.PkgPath()) && t.Name() != "" {
			b.queue = append(b.queue, t)
			*refs = append(*refs, t)
		}
	}
}

// topLevelValueDep returns the value-struct dependency a field imposes on
// class-definition order: non-nil only when the field embeds a kernel struct by
// value (not via pointer/slice/map). Mirrors the original Python generator's dep
// rule so Python output stays stable.
func topLevelValueDep(t reflect.Type) reflect.Type {
	if t.Kind() != reflect.Struct {
		return nil
	}
	if t.PkgPath() == "time" && t.Name() == "Time" {
		return nil
	}
	if IsKernelPkg(t.PkgPath()) && t.Name() != "" {
		return t
	}
	return nil
}

func appendUnique(dst []reflect.Type, more ...reflect.Type) []reflect.Type {
	for _, t := range more {
		found := false
		for _, e := range dst {
			if e == t {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, t)
		}
	}
	return dst
}

// topoSortStructs orders structs so that value-struct dependencies are defined
// before their dependents. Ties break alphabetically for deterministic output.
func topoSortStructs(in map[reflect.Type]*Struct) []*Struct {
	all := make([]*Struct, 0, len(in))
	byName := map[string]*Struct{}
	depNames := map[string][]string{}
	for _, sd := range in {
		all = append(all, sd)
		byName[sd.Name] = sd
		var deps []string
		for _, d := range sd.Deps {
			deps = append(deps, d.Name())
		}
		depNames[sd.Name] = deps
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	emitted := map[string]bool{}
	var ordered []*Struct
	// Deterministic Kahn-style: repeatedly emit the alphabetically-first node whose
	// value-struct deps are all already emitted. Falls back to forcing progress on a
	// cycle (value-struct cycles are impossible in valid Go, but stay safe).
	for len(ordered) < len(all) {
		progressed := false
		for _, sd := range all {
			if emitted[sd.Name] {
				continue
			}
			ready := true
			for _, dn := range depNames[sd.Name] {
				if _, known := byName[dn]; known && !emitted[dn] {
					ready = false
					break
				}
			}
			if ready {
				ordered = append(ordered, sd)
				emitted[sd.Name] = true
				progressed = true
			}
		}
		if !progressed {
			for _, sd := range all {
				if !emitted[sd.Name] {
					ordered = append(ordered, sd)
					emitted[sd.Name] = true
				}
			}
		}
	}
	return ordered
}

// OrderByReferences topo-sorts structs so that every referenced struct schema is
// declared before the struct that references it — required by emitters where a
// const must be initialized before use (TypeScript/Zod), unlike Python's
// forward-referencing class bodies. Ties break alphabetically for determinism;
// a reference cycle (impossible across the current governed set) falls back to
// forcing progress so output stays total.
func OrderByReferences(structs []*Struct) []*Struct {
	all := append([]*Struct(nil), structs...)
	byName := map[string]*Struct{}
	refNames := map[string][]string{}
	for _, sd := range all {
		byName[sd.Name] = sd
		var names []string
		for _, r := range sd.Refs {
			names = append(names, r.Name())
		}
		refNames[sd.Name] = names
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	emitted := map[string]bool{}
	var ordered []*Struct
	for len(ordered) < len(all) {
		progressed := false
		for _, sd := range all {
			if emitted[sd.Name] {
				continue
			}
			ready := true
			for _, rn := range refNames[sd.Name] {
				if rn == sd.Name {
					continue // self-reference handled at the field level
				}
				if _, known := byName[rn]; known && !emitted[rn] {
					ready = false
					break
				}
			}
			if ready {
				ordered = append(ordered, sd)
				emitted[sd.Name] = true
				progressed = true
			}
		}
		if !progressed {
			for _, sd := range all {
				if !emitted[sd.Name] {
					ordered = append(ordered, sd)
					emitted[sd.Name] = true
				}
			}
		}
	}
	return ordered
}

// extractEnums reads enum const declarations from Go source so emitted enum values
// stay faithful to the kernel. Returns a map keyed by "pkgPath.TypeName".
func extractEnums(root string) (map[string]*EnumInfo, error) {
	reg := map[string]*EnumInfo{}
	paths := make([]string, 0, len(PkgDirs))
	for p := range PkgDirs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, pkgPath := range paths {
		dir := filepath.Join(root, filepath.FromSlash(PkgDirs[pkgPath]))
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}
		files := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			files = append(files, e.Name())
		}
		sort.Strings(files)

		fset := token.NewFileSet()
		for _, name := range files {
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			if err != nil {
				return nil, fmt.Errorf("parsing %s: %w", name, err)
			}
			collectConstEnums(f, pkgPath, reg)
		}
	}
	return reg, nil
}

// collectConstEnums walks const declarations, grouping typed string/int constants
// into enums. Handles iota by tracking the implicit type/expression carry-over.
func collectConstEnums(f *ast.File, pkgPath string, reg map[string]*EnumInfo) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		var carryType ast.Expr
		var carryExpr ast.Expr
		for iota, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typ := vs.Type
			if typ == nil {
				typ = carryType
			} else {
				carryType = typ
			}
			var expr ast.Expr
			if len(vs.Values) == 1 {
				expr = vs.Values[0]
				carryExpr = expr
			} else {
				expr = carryExpr
			}
			carryType = typ // implicit specs inherit the most recent explicit type

			ident, ok := typ.(*ast.Ident)
			if !ok || ident.Name == "" {
				continue // untyped or qualified — not a local enum
			}
			key := pkgPath + "." + ident.Name

			for _, nameIdent := range vs.Names {
				if nameIdent.Name == "_" {
					continue
				}
				member, isString, ok := evalConst(expr, iota)
				if !ok {
					continue
				}
				info := reg[key]
				if info == nil {
					info = &EnumInfo{Name: ident.Name, IsString: isString}
					reg[key] = info
				}
				m := EnumMember{Name: MemberName(ident.Name, nameIdent.Name)}
				if isString {
					m.StrVal = member.(string)
				} else {
					m.IntVal = member.(int64)
				}
				info.Members = append(info.Members, m)
			}
		}
	}
}

// evalConst evaluates the small subset of constant expressions NAVI enums use:
// string literals and iota-based integers.
func evalConst(expr ast.Expr, iota int) (val any, isString bool, ok bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			s, err := strconv.Unquote(e.Value)
			if err != nil {
				return nil, false, false
			}
			return s, true, true
		case token.INT:
			n, err := strconv.ParseInt(e.Value, 0, 64)
			if err != nil {
				return nil, false, false
			}
			return n, false, true
		}
	case *ast.Ident:
		if e.Name == "iota" {
			return int64(iota), false, true
		}
	}
	return nil, false, false
}

// MemberName converts a Go const name into a SCREAMING_SNAKE_CASE enum member
// identifier, stripping the enum's type-name prefix when present for brevity.
func MemberName(typeName, constName string) string {
	name := constName
	if strings.HasPrefix(name, typeName) && len(name) > len(typeName) {
		name = name[len(typeName):]
	}
	return CamelToScreamingSnake(name)
}

// CamelToScreamingSnake converts a CamelCase identifier to SCREAMING_SNAKE_CASE.
func CamelToScreamingSnake(s string) string {
	var b strings.Builder
	prevLowerOrDigit := false
	for i, r := range s {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper && i > 0 && prevLowerOrDigit {
			b.WriteByte('_')
		}
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r - 32)
			prevLowerOrDigit = true
		} else if r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevLowerOrDigit = true
		} else if isUpper {
			b.WriteRune(r)
			prevLowerOrDigit = false
		} else {
			b.WriteByte('_')
			prevLowerOrDigit = false
		}
	}
	out := b.String()
	if out != "" && out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

// JSONName mirrors encoding/json's key derivation: the tag name when present, the
// Go field name otherwise; a "-" tag drops the field.
func JSONName(f reflect.StructField) (name string, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", true
	}
	if parts[0] == "" {
		return f.Name, false
	}
	return parts[0], false
}

// IsOmitEmpty reports whether a field's json tag carries the omitempty option,
// which renderers map to optional fields.
func IsOmitEmpty(f reflect.StructField) bool {
	tag := f.Tag.Get("json")
	if tag == "" {
		return false
	}
	for _, opt := range strings.Split(tag, ",")[1:] {
		if opt == "omitempty" {
			return true
		}
	}
	return false
}

// TypeKey returns the "pkgPath.TypeName" key for a named type, or "" otherwise.
func TypeKey(t reflect.Type) string {
	if t.Name() == "" || t.PkgPath() == "" {
		return ""
	}
	return t.PkgPath() + "." + t.Name()
}

// GoRef returns a short "pkg.TypeName" reference for docstrings.
func GoRef(t reflect.Type) string {
	pkg := t.PkgPath()
	short := pkg[strings.LastIndex(pkg, "/")+1:]
	return short + "." + t.Name()
}

// IsKernelPkg reports whether a package path is one of the governed kernel
// packages codegen sources from.
func IsKernelPkg(pkgPath string) bool {
	_, ok := PkgDirs[pkgPath]
	return ok
}
