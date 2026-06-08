package plugin

import "testing"

// TestManifestResolvesIDCoderAlias proves plugin-id alias resolution
// (migration ticket NEW-A): a navi.programmer manifest is addressable by the
// navi.coder alias and vice versa, while the unrelated real "navi-coder" plugin
// id resolves only itself.
func TestManifestResolvesIDCoderAlias(t *testing.T) {
	prog := Manifest{ID: "navi.programmer", PluginID: "navi.programmer"}
	if !prog.ResolvesID("navi.programmer") {
		t.Error("manifest must resolve its own id")
	}
	if !prog.ResolvesID("navi.coder") {
		t.Error("navi.programmer manifest must resolve the navi.coder alias")
	}
	if prog.ResolvesID("navi-coder") {
		t.Error("navi.programmer manifest must NOT resolve the unrelated navi-coder plugin id")
	}
	if prog.ResolvesID("") {
		t.Error("empty id must not resolve")
	}

	// Reverse direction: a manifest carrying the coder alias resolves the legacy id.
	coder := Manifest{ID: "navi.coder", PluginID: "navi.coder"}
	if !coder.ResolvesID("navi.programmer") {
		t.Error("navi.coder manifest must resolve the legacy navi.programmer id")
	}

	// The real, unrelated navi-coder plugin resolves only itself.
	real := Manifest{ID: "navi-coder", PluginID: "navi-coder"}
	if !real.ResolvesID("navi-coder") {
		t.Error("navi-coder manifest must resolve its own id")
	}
	if real.ResolvesID("navi.programmer") || real.ResolvesID("navi.coder") {
		t.Error("navi-coder must NOT resolve the programmer/coder-alias ids")
	}
}
