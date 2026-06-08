package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolveDaemonCommandExplicit(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, daemonBinaryNameForTest())
	writeFakeExecutable(t, bin)

	got, args, err := resolveDaemonCommand(bin, dir)
	if err != nil {
		t.Fatalf("resolve explicit daemon command: %v", err)
	}
	want, _ := filepath.Abs(bin)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if len(args) != 0 {
		t.Fatalf("expected no command args, got %v", args)
	}
}

func TestResolveDaemonCommandFindsCwdBin(t *testing.T) {
	cwd := t.TempDir()
	binDir := filepath.Join(cwd, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	bin := filepath.Join(binDir, daemonBinaryNameForTest())
	writeFakeExecutable(t, bin)

	got, _, err := resolveDaemonCommand("", cwd)
	if err != nil {
		t.Fatalf("resolve cwd daemon command: %v", err)
	}
	if got != bin {
		t.Fatalf("expected %q, got %q", bin, got)
	}
}

func TestResolveDaemonCommandPrefersWindowsExe(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific executable ordering")
	}
	cwd := t.TempDir()
	binDir := filepath.Join(cwd, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	extensionless := filepath.Join(binDir, "navid")
	windowsExe := filepath.Join(binDir, "navid.exe")
	writeFakeExecutable(t, extensionless)
	writeFakeExecutable(t, windowsExe)

	got, _, err := resolveDaemonCommand("", cwd)
	if err != nil {
		t.Fatalf("resolve cwd daemon command: %v", err)
	}
	if got != windowsExe {
		t.Fatalf("expected Windows executable %q, got %q", windowsExe, got)
	}
}

func TestDaemonStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths := daemonPaths{
		Dir:       dir,
		PIDPath:   filepath.Join(dir, "navid.pid"),
		StatePath: filepath.Join(dir, "navid.json"),
		LogPath:   filepath.Join(dir, "navid.log"),
	}
	state := daemonState{
		PID:        12345,
		StartedAt:  time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Command:    []string{"navid"},
		WorkDir:    "C:/repo/navi",
		GatewayURL: "http://localhost:6284",
		LogPath:    paths.LogPath,
	}
	if err := writeDaemonState(paths, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	got, err := readDaemonState(paths)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !reflect.DeepEqual(*got, state) {
		t.Fatalf("state mismatch\nwant: %#v\n got: %#v", state, *got)
	}
	pidBytes, err := os.ReadFile(paths.PIDPath)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	if string(pidBytes) != "12345\n" {
		t.Fatalf("unexpected pid file %q", string(pidBytes))
	}
}

func TestTailLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "navid.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	got, err := tailLines(path, 2)
	if err != nil {
		t.Fatalf("tail lines: %v", err)
	}
	want := []string{"three", "four"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestDaemonHealthURL(t *testing.T) {
	tests := map[string]string{
		"":                        "http://localhost:6284/health",
		"http://localhost:6284":   "http://localhost:6284/health",
		"http://localhost:6284/":  "http://localhost:6284/health",
		"http://127.0.0.1:6284//": "http://127.0.0.1:6284/health",
	}
	for input, want := range tests {
		if got := daemonHealthURL(input); got != want {
			t.Fatalf("daemonHealthURL(%q): expected %q, got %q", input, want, got)
		}
	}
}

func TestGatewayURLFromListenAddr(t *testing.T) {
	tests := map[string]string{
		"":                 "http://localhost:6284",
		":6285":            "http://127.0.0.1:6285",
		"127.0.0.1:6285":   "http://127.0.0.1:6285",
		"http://navi:6284": "http://navi:6284",
	}
	for input, want := range tests {
		if got := gatewayURLFromListenAddr(input); got != want {
			t.Fatalf("gatewayURLFromListenAddr(%q): expected %q, got %q", input, want, got)
		}
	}
}

func TestDaemonCommandEnvOverridesGatewayAddr(t *testing.T) {
	got := daemonCommandEnv([]string{"PATH=x", "NAVI_GATEWAY_ADDR=:6284"}, "127.0.0.1:6285")
	want := []string{"PATH=x", "NAVI_GATEWAY_ADDR=127.0.0.1:6285"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	got = daemonCommandEnv([]string{"PATH=x"}, ":6286")
	want = []string{"PATH=x", "NAVI_GATEWAY_ADDR=:6286"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected appended env %v, got %v", want, got)
	}
}

func TestDaemonControlRequiresPackagedLauncher(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		allowed bool
	}{
		{name: "missing channel", env: []string{"PATH=x"}, allowed: false},
		{name: "npm channel", env: []string{"NAVI_DISTRIBUTION_CHANNEL=npm"}, allowed: true},
		{name: "pip channel", env: []string{"NAVI_DISTRIBUTION_CHANNEL=pip"}, allowed: true},
		{name: "node alias", env: []string{"NAVI_DISTRIBUTION_CHANNEL=node"}, allowed: true},
		{name: "python alias", env: []string{"NAVI_DISTRIBUTION_CHANNEL=python"}, allowed: true},
		{name: "developer override", env: []string{"NAVI_DISTRIBUTION_CHANNEL=dev"}, allowed: true},
		{name: "unsupported channel", env: []string{"NAVI_DISTRIBUTION_CHANNEL=brew"}, allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := daemonControlAllowed(tt.env); got != tt.allowed {
				t.Fatalf("expected allowed=%v, got %v", tt.allowed, got)
			}
		})
	}
}

func TestRequirePackagedDaemonControlMessage(t *testing.T) {
	var out bytes.Buffer
	if requirePackagedDaemonControl(&out, []string{"PATH=x"}) {
		t.Fatal("expected direct daemon control to be rejected")
	}
	text := out.String()
	for _, want := range []string{"npm install open-navi", "pip install open-navi", "NAVI_DISTRIBUTION_CHANNEL=dev"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected message to mention %q, got %q", want, text)
		}
	}
}

func daemonBinaryNameForTest() string {
	if runtime.GOOS == "windows" {
		return "navid.exe"
	}
	return "navid"
}

func writeFakeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatalf("chmod fake executable: %v", err)
		}
	}
}
