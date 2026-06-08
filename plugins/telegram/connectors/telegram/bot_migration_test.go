package telegram

import (
	"testing"
)

func TestParseMediaMetadata(t *testing.T) {
	bot := &Bot{}

	tests := []struct {
		name     string
		msg      *tgMessage
		wantKind string
		wantSize int64
	}{
		{
			name: "photo",
			msg: &tgMessage{
				Photo: []tgPhotoSize{
					{FileID: "p1", FileSize: 100},
					{FileID: "p2", FileSize: 200},
				},
			},
			wantKind: "photo",
			wantSize: 200,
		},
		{
			name: "video",
			msg: &tgMessage{
				Video: &tgVideo{FileID: "v1", FileSize: 500},
			},
			wantKind: "video",
			wantSize: 500,
		},
		{
			name: "document",
			msg: &tgMessage{
				Document: &tgDocument{FileID: "d1", FileSize: 1000},
			},
			wantKind: "document",
			wantSize: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachments := bot.parseMediaMetadata(tt.msg)
			if len(attachments) != 1 {
				t.Fatalf("expected 1 attachment, got %d", len(attachments))
			}
			if attachments[0].MediaKind != tt.wantKind {
				t.Errorf("got Kind %s, want %s", attachments[0].MediaKind, tt.wantKind)
			}
			if attachments[0].SizeBytes != tt.wantSize {
				t.Errorf("got Size %d, want %d", attachments[0].SizeBytes, tt.wantSize)
			}
		})
	}
}
