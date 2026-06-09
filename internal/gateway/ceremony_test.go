package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/navi/experience"
	"github.com/open-navi/navi/internal/onboarding"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func TestCeremonyRoutesAfterRequiredOnboardingUntilResolved(t *testing.T) {
	db := testDB(t)
	dataDir := t.TempDir()
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>TEST_CONSOLE_CONTENT</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	fakeLLM := &onboardingFakeLLM{}
	srv := NewServer(Config{
		DB:        db,
		DataDir:   dataDir,
		StaticDir: staticDir,
		Addr:      ":0",
		LLM:       fakeLLM,
	})

	res := doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Ceremony Route Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/provider", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST provider: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/connection", "", "", "127.0.0.1:1234", map[string]any{"skip": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST connection skip: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/complete", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST complete: expected 200, got %d", res.StatusCode)
	}
	var completeResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&completeResp); err != nil {
		t.Fatalf("decode complete: %v", err)
	}
	// After required onboarding, redirect is "/" — frontend calls /api/ceremony/init-chat to create the chat.
	if completeResp["redirect"] != "/" {
		t.Fatalf("expected required onboarding to redirect to /, got %v", completeResp["redirect"])
	}

	// Root page should serve the console (no server-side ceremony redirect — frontend handles routing).
	root := doReq(t, srv, http.MethodGet, "/", "", "", "127.0.0.1:1234", nil)
	if root.StatusCode != http.StatusOK {
		t.Fatalf("root before ceremony should serve console (frontend routes to init-chat), got %d", root.StatusCode)
	}
	ceremonyPage := doReq(t, srv, http.MethodGet, "/ceremony", "", "", "127.0.0.1:1234", nil)
	if ceremonyPage.StatusCode != http.StatusOK {
		t.Fatalf("/ceremony should serve console shell, got %d", ceremonyPage.StatusCode)
	}

	skip := doReq(t, srv, http.MethodPost, "/api/ceremony/skip", "", "", "127.0.0.1:1234", nil)
	if skip.StatusCode != http.StatusOK {
		t.Fatalf("POST ceremony skip: expected 200, got %d", skip.StatusCode)
	}
	var skipResp map[string]any
	if err := json.NewDecoder(skip.Body).Decode(&skipResp); err != nil {
		t.Fatalf("decode skip: %v", err)
	}
	journeyState := skipResp["journeyState"].(map[string]any)
	if journeyState["status"] != "skipped" || skipResp["redirect"] != "/" {
		t.Fatalf("unexpected skip response: %#v", skipResp)
	}

	root = doReq(t, srv, http.MethodGet, "/", "", "", "127.0.0.1:1234", nil)
	if root.StatusCode != http.StatusOK {
		t.Fatalf("root after skipped ceremony should serve console, got %d", root.StatusCode)
	}
}

func TestCeremonyStatusDoesNotPrefillOwnerSetupName(t *testing.T) {
	srv, _, db := testServer(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Administrative Owner")

	res := doReq(t, srv, http.MethodGet, "/api/ceremony", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET ceremony: expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode ceremony: %v", err)
	}
	journeyState := body["journeyState"].(map[string]any)
	if journeyState["status"] != "not_started" {
		t.Fatalf("expected not_started ceremony, got %#v", journeyState)
	}
	ownerSeed := body["ownerProfileSeed"].(map[string]any)
	if ownerSeed["displayName"] != "" {
		t.Fatalf("ceremony must not prefill from setup owner name, got %#v", ownerSeed)
	}
	if strings.Contains(string(mustMarshalCeremonyTest(t, body)), "Administrative Owner") {
		t.Fatalf("ceremony status leaked setup owner name into relationship state: %#v", body)
	}
}

func TestCeremonyCompletePersistsPresenceBoundariesAndCommunicationSeed(t *testing.T) {
	srv, _, db := testServer(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Administrative Owner")
	owner, _, err := store.GetOwner(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwner: %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/complete", "", "", "127.0.0.1:1234", map[string]any{
		"display_name":  "Eric",
		"presence_mode": "direct_strategic",
		"trust_boundaries": map[string]any{
			"confirm_before_sending_messages": true,
			"confirm_before_changing_files":   true,
		},
		"personalization_seed": "I prefer direct answers.",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST ceremony complete: expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode complete: %v", err)
	}
	journeyState := body["journeyState"].(map[string]any)
	if journeyState["status"] != "completed" || body["redirect"] != "/" {
		t.Fatalf("unexpected complete response: %#v", body)
	}
	if _, found, err := store.GetSetting(context.Background(), db, "ceremony_pact"); err != nil || found {
		t.Fatalf("pact summary must not be persisted as a standalone setting, found=%v err=%v", found, err)
	}

	storedOwner, _, err := store.GetOwner(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwner after ceremony: %v", err)
	}
	if storedOwner.Name != "Administrative Owner" {
		t.Fatalf("ceremony should not rewrite required-onboarding owner name, got %q", storedOwner.Name)
	}
	contact, err := store.GetContact(context.Background(), db, owner.ID)
	if err != nil {
		t.Fatalf("GetContact(owner): %v", err)
	}
	if contact.Name != "Eric" || !strings.Contains(contact.Metadata, `"created_from":"navi_ceremony"`) || !strings.Contains(contact.Metadata, `"owner_set":true`) {
		t.Fatalf("owner contact was not seeded from ceremony: %+v", contact)
	}

	snapshot, err := experience.LoadConfigurationSnapshot(context.Background(), db, owner.ID)
	if err != nil {
		t.Fatalf("LoadConfigurationSnapshot: %v", err)
	}
	if got := snapshot.CoreIdentity.Traits["directness"]; got < 0.78 {
		t.Fatalf("direct_strategic should raise directness, got %.2f", got)
	}
	if got := snapshot.CoreIdentity.Traits["challenge_intensity"]; got < 0.62 {
		t.Fatalf("direct_strategic should support strategic challenge, got %.2f", got)
	}

	rawBoundaries, ok, err := store.GetConfigurationValue(context.Background(), db, "owner", owner.ID, "ceremony.trust_boundaries.v1")
	if err != nil || !ok {
		t.Fatalf("trust boundary config missing: ok=%v err=%v", ok, err)
	}
	var boundaries map[string]any
	if err := json.Unmarshal([]byte(rawBoundaries), &boundaries); err != nil {
		t.Fatalf("decode trust boundaries: %v", err)
	}
	if boundaries["created_from"] != "navi_ceremony" || boundaries["owner_set"] != true || boundaries["confirm_before_changing_files"] != true {
		t.Fatalf("unexpected trust boundaries: %#v", boundaries)
	}

	rawSeed, ok, err := store.GetConfigurationValue(context.Background(), db, "owner", owner.ID, "ceremony.personalization_seed.v1")
	if err != nil || !ok {
		t.Fatalf("personalization seed config missing: ok=%v err=%v", ok, err)
	}
	var seed map[string]any
	if err := json.Unmarshal([]byte(rawSeed), &seed); err != nil {
		t.Fatalf("decode personalization seed: %v", err)
	}
	if seed["routed_to"] != "experience_preference" || seed["reviewable"] != true {
		t.Fatalf("communication preference seed should route to experience preference and remain reviewable: %#v", seed)
	}
	pactSummary := body["pactSummary"].([]any)
	if !containsCeremonyPactLine(pactSummary, "I'll remember I prefer direct answers.") {
		t.Fatalf("pact summary should avoid duplicate punctuation, got %#v", pactSummary)
	}
	signals, err := store.ListPreferenceSignalsByScope(context.Background(), db, "owner", owner.ID, nil, 20)
	if err != nil {
		t.Fatalf("ListPreferenceSignalsByScope: %v", err)
	}
	if !hasCeremonySignal(signals, "directness") {
		t.Fatalf("expected directness preference signal from communication seed, got %+v", signals)
	}
}

func TestCeremonyConservativeSeedRoutingStagesSensitiveInput(t *testing.T) {
	srv, _, db := testServer(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Administrative Owner")
	owner, _, err := store.GetOwner(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwner: %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/complete", "", "", "127.0.0.1:1234", map[string]any{
		"display_name":         "Eric",
		"presence_mode":        "balanced",
		"personalization_seed": "I am working through burnout.",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST ceremony complete: expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode complete: %v", err)
	}
	seed := body["personalizationSeed"].(map[string]any)
	if seed["routedTo"] != "review_candidate" || seed["reviewable"] != true {
		t.Fatalf("sensitive seed should stage as review candidate: %#v", seed)
	}
	memories, err := store.ListMemories(context.Background(), db, "owner", owner.ID, 10)
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != 0 {
		t.Fatalf("sensitive ceremony seed must not be silently stored as memory, got %+v", memories)
	}
}

func completeRequiredOnboardingForCeremonyTest(t *testing.T, srv *Server, db *sql.DB, ownerName string) {
	t.Helper()
	claimTestInstance(t, srv, ownerName, "owner-secret")
	ctx := context.Background()
	for key, value := range map[string]string{
		onboarding.SettingKeySetupComplete:  "true",
		onboarding.SettingKeyComplete:       "true",
		onboarding.SettingKeyOnboardingMode: "false",
		onboarding.SettingKeyFirstRunState:  string(onboarding.FirstRunComplete),
	} {
		if err := store.SetSetting(ctx, db, key, value); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
}

func mustMarshalCeremonyTest(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func hasCeremonySignal(signals []schema.PreferenceSignal, trait string) bool {
	for _, signal := range signals {
		if signal.Trait == trait && strings.Contains(signal.Summary, "navi_ceremony") {
			return true
		}
	}
	return false
}

func containsCeremonyPactLine(lines []any, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func TestInitCeremonyChat_CreatesChat(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST init-chat: expected 201, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode init-chat: %v", err)
	}
	chatID, _ := body["chatId"].(string)
	if chatID == "" {
		t.Fatalf("expected chatId in response, got %#v", body)
	}
	if body["redirect"] != "/chats/"+chatID {
		t.Fatalf("expected redirect /chats/%s, got %v", chatID, body["redirect"])
	}

	state, err := onboarding.LoadCeremonyJourneyState(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadCeremonyJourneyState: %v", err)
	}
	if state.ChatID != chatID {
		t.Fatalf("ceremony state ChatID not set: got %q, want %q", state.ChatID, chatID)
	}
	if state.CurrentStep != "owner_recognition" {
		t.Fatalf("expected step owner_recognition, got %q", state.CurrentStep)
	}
}

func TestInitCeremonyChat_Idempotent(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("first init-chat: expected 201, got %d", res.StatusCode)
	}
	var first map[string]any
	json.NewDecoder(res.Body).Decode(&first)
	chatID := first["chatId"].(string)

	res2 := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("second init-chat: expected 200, got %d", res2.StatusCode)
	}
	var second map[string]any
	json.NewDecoder(res2.Body).Decode(&second)
	if second["chatId"] != chatID {
		t.Fatalf("idempotent init-chat returned different chatId: %v vs %v", second["chatId"], chatID)
	}
}

func TestCeremonyOwnerRecognition_EmitsUserVisibleLiveEvents(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("init-chat: expected 201, got %d", res.StatusCode)
	}
	var initBody map[string]any
	if err := json.NewDecoder(res.Body).Decode(&initBody); err != nil {
		t.Fatalf("decode init-chat: %v", err)
	}
	chatID, _ := initBody["chatId"].(string)
	if chatID == "" {
		t.Fatalf("expected chatId in init-chat response")
	}

	res = doReq(t, srv, http.MethodPost, "/api/navi/chats/"+chatID+"/message", "", "", "127.0.0.1:1234", map[string]any{
		"content":        "EJ",
		"source_channel": "web",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("ceremony owner message: expected 201, got %d", res.StatusCode)
	}

	// Ceremony assistant turns stream asynchronously (partials → completed → run.completed).
	time.Sleep(400 * time.Millisecond)

	events, err := store.SessionEventsSince(
		context.Background(),
		db,
		chatID,
		0,
		[]schema.EventVisibility{schema.VisibilityUser},
		100,
	)
	if err != nil {
		t.Fatalf("SessionEventsSince: %v", err)
	}
	var sawPartial, sawPresencePrompt, sawRunCompleted bool
	for _, ev := range events {
		if ev.Visibility != schema.VisibilityUser {
			t.Fatalf("expected user_visible event, got visibility %q for type %s", ev.Visibility, ev.Type)
		}
		switch ev.Type {
		case schema.FactAssistantMessagePartial:
			sawPartial = true
		case schema.FactAssistantMessageCompleted:
			payloadJSON, err := json.Marshal(ev.Payload)
			if err != nil {
				t.Fatalf("marshal assistant completed payload: %v", err)
			}
			var payload schema.AssistantMessageCompletedPayload
			if err := json.Unmarshal(payloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal assistant completed payload: %v", err)
			}
			if strings.Contains(payload.Content, "How would you prefer") {
				sawPresencePrompt = true
			}
		case schema.FactRunCompleted:
			sawRunCompleted = true
		}
	}
	if !sawPartial {
		t.Fatal("expected user-visible assistant.message.partial while streaming ceremony reply")
	}
	if !sawPresencePrompt {
		t.Fatal("expected user-visible assistant.message.completed with presence step prompt after owner_recognition")
	}
	if !sawRunCompleted {
		t.Fatal("expected user-visible run.completed after ceremony owner_recognition")
	}
}

func TestCeremonyStep_PresenceChoice(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	var initBody map[string]any
	json.NewDecoder(res.Body).Decode(&initBody)
	chatID := initBody["chatId"].(string)

	if _, err := onboarding.StartCeremonyJourney(context.Background(), db, "navi_presence"); err != nil {
		t.Fatalf("start journey at navi_presence: %v", err)
	}

	res = doReq(t, srv, http.MethodPost, "/api/ceremony/step", "", "", "127.0.0.1:1234", map[string]any{
		"chatId": chatID,
		"step":   "navi_presence",
		"value":  "direct_strategic",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ceremony step presence: expected 200, got %d", res.StatusCode)
	}

	state, _ := onboarding.LoadCeremonyJourneyState(context.Background(), db)
	if state.CurrentStep != "trust_boundaries" {
		t.Fatalf("expected step trust_boundaries after presence, got %q", state.CurrentStep)
	}
}

func TestCeremonyStep_TrustChoice(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	var initBody map[string]any
	json.NewDecoder(res.Body).Decode(&initBody)
	chatID := initBody["chatId"].(string)

	if _, err := onboarding.StartCeremonyJourney(context.Background(), db, "trust_boundaries"); err != nil {
		t.Fatalf("start journey at trust_boundaries: %v", err)
	}

	res = doReq(t, srv, http.MethodPost, "/api/ceremony/step", "", "", "127.0.0.1:1234", map[string]any{
		"chatId": chatID,
		"step":   "trust_boundaries",
		"value": map[string]bool{
			"confirm_before_changing_files": true,
			"confirm_before_purchases":      true,
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ceremony step trust: expected 200, got %d", res.StatusCode)
	}

	state, _ := onboarding.LoadCeremonyJourneyState(context.Background(), db)
	if state.CurrentStep != "personalization_seed" {
		t.Fatalf("expected step personalization_seed after trust, got %q", state.CurrentStep)
	}
}

func TestCeremonyStep_PactConfirm(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	var initBody map[string]any
	json.NewDecoder(res.Body).Decode(&initBody)
	chatID := initBody["chatId"].(string)

	if _, err := onboarding.StartCeremonyJourney(context.Background(), db, "pact_summary"); err != nil {
		t.Fatalf("start journey at pact_summary: %v", err)
	}

	res = doReq(t, srv, http.MethodPost, "/api/ceremony/step", "", "", "127.0.0.1:1234", map[string]any{
		"chatId": chatID,
		"step":   "pact_summary",
		"action": "confirm",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ceremony step pact confirm: expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(res.Body).Decode(&body)
	if body["ok"] != true {
		t.Fatalf("expected ok true, got %v", body["ok"])
	}
	if _, hasRedirect := body["redirect"]; hasRedirect {
		t.Fatalf("expected no redirect on pact confirm, got %v", body["redirect"])
	}

	state, _ := onboarding.LoadCeremonyJourneyState(context.Background(), db)
	if state.Status != onboarding.CeremonyStatusCompleted {
		t.Fatalf("expected ceremony completed, got %q", state.Status)
	}

	time.Sleep(400 * time.Millisecond)

	events, err := store.SessionEventsSince(
		context.Background(),
		db,
		chatID,
		0,
		[]schema.EventVisibility{schema.VisibilityUser},
		100,
	)
	if err != nil {
		t.Fatalf("SessionEventsSince: %v", err)
	}
	var sawClosing bool
	for _, ev := range events {
		if ev.Type != schema.FactAssistantMessageCompleted {
			continue
		}
		payloadJSON, err := json.Marshal(ev.Payload)
		if err != nil {
			continue
		}
		var payload schema.AssistantMessageCompletedPayload
		if err := json.Unmarshal(payloadJSON, &payload); err != nil {
			continue
		}
		if strings.Contains(payload.Content, "I'm ready") {
			sawClosing = true
			break
		}
	}
	if !sawClosing {
		t.Fatal("expected user-visible closing assistant message after pact confirm")
	}

	chatRes := doReq(t, srv, http.MethodGet, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", nil)
	if chatRes.StatusCode != http.StatusOK {
		t.Fatalf("GET chat: expected 200, got %d", chatRes.StatusCode)
	}
	var chatBody map[string]any
	if err := json.NewDecoder(chatRes.Body).Decode(&chatBody); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	messages, _ := chatBody["messages"].([]any)
	var sawUserConfirm, sawClosingMessage bool
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		content, _ := msg["content"].(string)
		role, _ := msg["role"].(string)
		if role == "user" && content == "Looks right" {
			sawUserConfirm = true
		}
		if role == "assistant" && strings.Contains(content, "I'm ready") {
			sawClosingMessage = true
		}
	}
	if !sawUserConfirm {
		t.Fatal("expected user transcript line for pact confirm")
	}
	if !sawClosingMessage {
		t.Fatal("expected closing assistant message in chat transcript")
	}
}

func TestCeremonyStep_PactConfirmRejectedWhenNotInProgress(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	var initBody map[string]any
	json.NewDecoder(res.Body).Decode(&initBody)
	chatID := initBody["chatId"].(string)

	if _, err := onboarding.StartCeremonyJourney(context.Background(), db, "pact_summary"); err != nil {
		t.Fatalf("start journey at pact_summary: %v", err)
	}

	res = doReq(t, srv, http.MethodPost, "/api/ceremony/step", "", "", "127.0.0.1:1234", map[string]any{
		"chatId": chatID,
		"step":   "pact_summary",
		"action": "confirm",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("first pact confirm: expected 200, got %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodPost, "/api/ceremony/step", "", "", "127.0.0.1:1234", map[string]any{
		"chatId": chatID,
		"step":   "pact_summary",
		"action": "confirm",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("second pact confirm: expected 400, got %d", res.StatusCode)
	}
}

func TestCeremonyStep_PactSkip(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	completeRequiredOnboardingForCeremonyTest(t, srv, db, "Test Owner")

	res := doReq(t, srv, http.MethodPost, "/api/ceremony/init-chat", "", "", "127.0.0.1:1234", nil)
	var initBody map[string]any
	json.NewDecoder(res.Body).Decode(&initBody)
	chatID := initBody["chatId"].(string)

	if _, err := onboarding.StartCeremonyJourney(context.Background(), db, "pact_summary"); err != nil {
		t.Fatalf("start journey at pact_summary: %v", err)
	}

	res = doReq(t, srv, http.MethodPost, "/api/ceremony/step", "", "", "127.0.0.1:1234", map[string]any{
		"chatId": chatID,
		"step":   "pact_summary",
		"action": "skip",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ceremony step pact skip: expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(res.Body).Decode(&body)
	if body["ok"] != true {
		t.Fatalf("expected ok true, got %v", body["ok"])
	}
	if _, hasRedirect := body["redirect"]; hasRedirect {
		t.Fatalf("expected no redirect on pact skip, got %v", body["redirect"])
	}

	state, _ := onboarding.LoadCeremonyJourneyState(context.Background(), db)
	if state.Status != onboarding.CeremonyStatusSkipped {
		t.Fatalf("expected ceremony skipped, got %q", state.Status)
	}

	time.Sleep(400 * time.Millisecond)

	chatRes := doReq(t, srv, http.MethodGet, "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", nil)
	if chatRes.StatusCode != http.StatusOK {
		t.Fatalf("GET chat: expected 200, got %d", chatRes.StatusCode)
	}
	var chatBody map[string]any
	if err := json.NewDecoder(chatRes.Body).Decode(&chatBody); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	messages, _ := chatBody["messages"].([]any)
	var sawSkipClosing bool
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		content, _ := msg["content"].(string)
		role, _ := msg["role"].(string)
		if role == "assistant" && strings.Contains(content, "revisit these choices") {
			sawSkipClosing = true
			break
		}
	}
	if !sawSkipClosing {
		t.Fatal("expected skip closing assistant message in chat transcript")
	}
}
