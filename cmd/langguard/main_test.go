package main

import "testing"

func TestImportedTopModule(t *testing.T) {
	cases := []struct {
		line   string
		want   string
		wantOK bool
	}{
		{"import sqlite3", "sqlite3", true},
		{"import a.b as c", "a", true},
		{"import os, sys", "os", true},
		{"from store.x import y", "store", true},
		{"  from connectors import telegram", "connectors", true},
		{"from . import _schema", "", false}, // relative — in-package
		{"from .client import query_context", "", false},
		{"    x = 1  # import sqlite3", "", false}, // not an import statement
		{"async def query_context(...):", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := importedTopModule(c.line)
		if got != c.want || ok != c.wantOK {
			t.Errorf("importedTopModule(%q) = (%q, %v); want (%q, %v)", c.line, got, ok, c.want, c.wantOK)
		}
	}
}

func TestForbiddenImportsDenylist(t *testing.T) {
	// The planted-violation contract case (`import store`) must be denied.
	if _, denied := forbiddenImports["store"]; !denied {
		t.Fatal("`store` must be on the forbidden-import denylist")
	}
	if _, denied := forbiddenImports["json"]; denied {
		t.Fatal("`json` (stdlib) must not be denied")
	}
}

func TestFindCalls(t *testing.T) {
	src := `
async def query_context(self, *, run_id, purpose, scope):  # definition, skipped
    pass

await client.query_context(run_id="r", purpose="eval_scoring", scope="current_run_summary")
await other.query_context(run_id="r", scope="current_run_summary")  # missing purpose
x = my_query_context(run_id="r")  # longer identifier, not our call
`
	calls := findCalls(src, "query_context")
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls (definition and my_query_context excluded), got %d: %+v", len(calls), calls)
	}
	if !contains(calls[0].args, "purpose") || !contains(calls[0].args, "scope") {
		t.Errorf("first call should carry purpose and scope: %q", calls[0].args)
	}
	if contains(calls[1].args, "purpose") {
		t.Errorf("second call should be missing purpose: %q", calls[1].args)
	}
}

func TestBalancedParens(t *testing.T) {
	src := `foo(a, bar(b, c), d)`
	open := len("foo")
	got, ok := balancedParens(src, open)
	if !ok || got != "a, bar(b, c), d" {
		t.Errorf("balancedParens = (%q, %v); want (%q, true)", got, ok, "a, bar(b, c), d")
	}
	if _, ok := balancedParens("foo(unterminated", 3); ok {
		t.Error("balancedParens should report ok=false for unbalanced parens")
	}
}

func TestNormalizeEOL(t *testing.T) {
	if normalizeEOL([]byte("a\r\nb\r\n")) != normalizeEOL([]byte("a\nb\n")) {
		t.Error("CRLF and LF content must normalize equal")
	}
	if normalizeEOL([]byte("a\nb")) != "a\nb" {
		t.Error("LF content must be unchanged")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
