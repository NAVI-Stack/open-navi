package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-navi/navi/internal/llm"
)

// privacyPrefsLLM is an onboardingFakeLLM that reports a configured privacy mode.
type privacyPrefsLLM struct {
	*onboardingFakeLLM
	mode llm.PrivacyMode
}

func (f *privacyPrefsLLM) GetPreferences(context.Context) (llm.ModelPreferences, error) {
	return llm.ModelPreferences{PrivacyMode: f.mode}, nil
}

// TestLLMProfiles_SurfacesPrivacyPolicy asserts GET /api/llm/profiles exposes the
// CIP §9 privacy-tier policy so operators can inspect it without code changes.
func TestLLMProfiles_SurfacesPrivacyPolicy(t *testing.T) {
	srv := NewServer(Config{
		LLM:  &privacyPrefsLLM{onboardingFakeLLM: &onboardingFakeLLM{}, mode: llm.PrivacyModeHybrid},
		Addr: ":0",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/llm/profiles", nil)
	req = req.WithContext(context.WithValue(req.Context(), scopesKey, []string{"read"}))
	w := httptest.NewRecorder()
	srv.handleLLMProfiles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Privacy llm.PrivacyPolicyView `json:"privacy"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Privacy.Mode != string(llm.PrivacyModeHybrid) {
		t.Errorf("privacy mode = %q, want hybrid", body.Privacy.Mode)
	}
	got := map[string]bool{}
	for _, c := range body.Privacy.Classes {
		got[c.Class] = c.CloudEligible
	}
	if got["secret"] {
		t.Error("secret must not be cloud-eligible in hybrid mode")
	}
	if !got["public"] {
		t.Error("public must be cloud-eligible in hybrid mode")
	}
}
