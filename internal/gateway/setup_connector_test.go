package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestHandleSetupConnectorPassesTelegramOptionalParams(t *testing.T) {
	srv, _, _ := testServer(t)

	called := false
	srv.cfg.OnSetupConnector = func(ctx context.Context, connType string, params map[string]string) error {
		called = true
		if connType != "telegram" {
			t.Fatalf("unexpected connector type %q", connType)
		}
		if params["account"] != "ops" {
			t.Fatalf("unexpected account %q", params["account"])
		}
		if params["allow_from"] != "100,200" {
			t.Fatalf("unexpected allow_from %q", params["allow_from"])
		}
		if params["pairing_code"] != "secret-code" {
			t.Fatalf("unexpected pairing_code %q", params["pairing_code"])
		}
		if params["gateway_url"] != "http://localhost:6284" {
			t.Fatalf("unexpected gateway_url %q", params["gateway_url"])
		}
		if params["api_url"] != "https://api.telegram.org" {
			t.Fatalf("unexpected api_url %q", params["api_url"])
		}
		if params["webhook_url"] != "https://example.com/webhook" {
			t.Fatalf("unexpected webhook_url %q", params["webhook_url"])
		}
		if params["webhook_secret"] != "hook-secret" {
			t.Fatalf("unexpected webhook_secret %q", params["webhook_secret"])
		}
		return nil
	}

	res := doJSONReq(t, srv, http.MethodPost, "/api/setup/connector", "", "127.0.0.1:1234", map[string]any{
		"type": "telegram",
		"params": map[string]string{
			"bot_token":      "token",
			"owner_chat_id":  "42",
			"account":        "ops",
			"allow_from":     "100,200",
			"pairing_code":   "secret-code",
			"gateway_url":    "http://localhost:6284",
			"api_url":        "https://api.telegram.org",
			"webhook_url":    "https://example.com/webhook",
			"webhook_secret": "hook-secret",
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if !called {
		t.Fatal("expected setup callback to be invoked")
	}
}

func TestGatewayRejectsInvalidDirectiveModes(t *testing.T) {
	srv, _, _ := testServer(t)

	res := doJSONReq(t, srv, http.MethodPost, "/api/directives", "", "127.0.0.1:1234", map[string]string{
		"title": "Bad Directive",
		"mode":  "DISCUSS",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}

	var errResp map[string]string
	if err := json.NewDecoder(res.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Fatal("expected invalid mode error message")
	}

	create := doJSONReq(t, srv, http.MethodPost, "/api/directives", "", "127.0.0.1:1234", map[string]string{
		"title": "Good Directive",
		"mode":  "ADVISE",
	})
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", create.StatusCode)
	}
	var d map[string]any
	if err := json.NewDecoder(create.Body).Decode(&d); err != nil {
		t.Fatalf("decode directive response: %v", err)
	}
	directiveID, _ := d["directive_id"].(string)
	if directiveID == "" {
		t.Fatal("expected directive_id in create response")
	}

	res = doJSONReq(t, srv, http.MethodPut, "/api/directives/"+directiveID+"/mode", "", "127.0.0.1:1234", map[string]string{
		"mode": "IMPLEMENT",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mode update, got %d", res.StatusCode)
	}
}
