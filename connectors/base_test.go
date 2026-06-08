package connectors

import (
	"testing"
)

func TestNewBaseConnector(t *testing.T) {
	name := "test-connector"
	allowList := []string{"user1", "user2"}
	bc := NewBaseConnector(name, allowList)

	if bc.name != name {
		t.Errorf("expected name %q, got %q", name, bc.name)
	}
	if len(bc.allowList) != len(allowList) {
		t.Errorf("expected allowList length %d, got %d", len(allowList), len(bc.allowList))
	}
	for i, v := range allowList {
		if bc.allowList[i] != v {
			t.Errorf("expected allowList[%d] %q, got %q", i, v, bc.allowList[i])
		}
	}
}

func TestBaseConnector_Name(t *testing.T) {
	name := "test-connector"
	bc := NewBaseConnector(name, nil)
	if bc.Name() != name {
		t.Errorf("expected Name() %q, got %q", name, bc.Name())
	}
}

func TestBaseConnector_Running(t *testing.T) {
	bc := NewBaseConnector("test", nil)

	if bc.IsRunning() {
		t.Error("expected IsRunning() to be false initially")
	}

	bc.SetRunning(true)
	if !bc.IsRunning() {
		t.Error("expected IsRunning() to be true after SetRunning(true)")
	}

	bc.SetRunning(false)
	if bc.IsRunning() {
		t.Error("expected IsRunning() to be false after SetRunning(false)")
	}
}

func TestBaseConnector_IsAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		senderID  string
		want      bool
	}{
		{
			name:      "empty allow-list",
			allowList: nil,
			senderID:  "anyone",
			want:      true,
		},
		{
			name:      "direct match",
			allowList: []string{"user1", "user2"},
			senderID:  "user1",
			want:      true,
		},
		{
			name:      "no match",
			allowList: []string{"user1", "user2"},
			senderID:  "user3",
			want:      false,
		},
		{
			name:      "match with @ prefix trimming",
			allowList: []string{"@user1"},
			senderID:  "user1",
			want:      true,
		},
		{
			name:      "match with @ prefix in both",
			allowList: []string{"@user1"},
			senderID:  "@user1",
			want:      true,
		},
		{
			name:      "senderID with pipe - match idPart",
			allowList: []string{"chan1"},
			senderID:  "chan1|user1",
			want:      true,
		},
		{
			name:      "senderID with pipe - match userPart",
			allowList: []string{"user1"},
			senderID:  "chan1|user1",
			want:      true,
		},
		{
			name:      "senderID with pipe - no match",
			allowList: []string{"other"},
			senderID:  "chan1|user1",
			want:      false,
		},
		{
			name:      "match trimmed allowList entry against idPart",
			allowList: []string{"@chan1"},
			senderID:  "chan1|user1",
			want:      true,
		},
		{
			name:      "match trimmed allowList entry against userPart",
			allowList: []string{"@user1"},
			senderID:  "chan1|user1",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := NewBaseConnector("test", tt.allowList)
			if got := bc.IsAllowed(tt.senderID); got != tt.want {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.senderID, got, tt.want)
			}
		})
	}
}
