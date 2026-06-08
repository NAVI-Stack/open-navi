package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/open-navi/navi/internal/navi"
	"github.com/open-navi/navi/internal/store"
)

func TestNaviRenameChat(t *testing.T) {
	srv, naviAgent, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Rename Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}

	ctx := context.Background()
	chatID, err := naviAgent.CreateChat(ctx, navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}

	// 1. Valid title update
	newTitle := "My Renamed Chat"
	res := doReq(t, srv, http.MethodPatch, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", map[string]any{
		"title": newTitle,
	})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 NoContent, got %d", res.StatusCode)
	}

	// Verify renamed title in GET /api/navi/chats/{id}
	res = doReq(t, srv, http.MethodGet, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET chat: expected 200, got %d", res.StatusCode)
	}
	var chat map[string]any
	if err := json.NewDecoder(res.Body).Decode(&chat); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	chatBody, _ := chat["chat"].(map[string]any)
	if chatBody["title"] != newTitle {
		t.Fatalf("expected title %q, got %q", newTitle, chatBody["title"])
	}

	// 2. Trimmed title persistence
	paddedTitle := "  Trimmed Title  "
	expectedTitle := "Trimmed Title"
	res = doReq(t, srv, http.MethodPatch, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", map[string]any{
		"title": paddedTitle,
	})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 NoContent for padded title, got %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodGet, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", nil)
	json.NewDecoder(res.Body).Decode(&chat)
	chatBody, _ = chat["chat"].(map[string]any)
	if chatBody["title"] != expectedTitle {
		t.Fatalf("expected trimmed title %q, got %q", expectedTitle, chatBody["title"])
	}

	// 3. Empty title rejected
	res = doReq(t, srv, http.MethodPatch, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", map[string]any{
		"title": "",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty title, got %d", res.StatusCode)
	}

	// 4. Whitespace-only title rejected
	res = doReq(t, srv, http.MethodPatch, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", map[string]any{
		"title": "   ",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace-only title, got %d", res.StatusCode)
	}

	// 5. Unknown session rejected
	res = doReq(t, srv, http.MethodPatch, "/api/navi/chats/non-existent-id", "", "", "127.0.0.1:1234", map[string]any{
		"title": "New Title",
	})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown session, got %d", res.StatusCode)
	}

	// 6. Verify renamed title appears in GET /api/navi/chats
	res = doReq(t, srv, http.MethodGet, "/api/navi/chats", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET chats list: expected 200, got %d", res.StatusCode)
	}
	var chats []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&chats); err != nil {
		t.Fatalf("decode chats list: %v", err)
	}
	found := false
	for _, c := range chats {
		if c["id"] == chatID {
			if c["title"] == expectedTitle {
				found = true
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected title %q to be in chats list for chat %s", expectedTitle, chatID)
	}
}
