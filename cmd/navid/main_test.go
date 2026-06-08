package main

import "testing"

func TestLocalGatewayURL(t *testing.T) {
	tests := map[string]string{
		"":                 "http://localhost:6284",
		":6284":            "http://localhost:6284",
		"127.0.0.1:6285":   "http://127.0.0.1:6285",
		"http://navi:6284": "http://navi:6284",
	}
	for input, want := range tests {
		if got := localGatewayURL(input); got != want {
			t.Fatalf("localGatewayURL(%q): expected %q, got %q", input, want, got)
		}
	}
}
