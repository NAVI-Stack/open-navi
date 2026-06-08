package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestExperienceInspect(t *testing.T) {
	srv, _, db := testServer(t)
	claimTestInstance(t, srv, "Inspect Owner", "owner-secret")

	ctx := context.Background()
	ownerID, err := store.GetOwnerID(ctx, db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}

	// Setup mock builder in server config
	srv.cfg.ExperienceBuilder = func(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error) {
		return experience.RenderedControl{
			State: experience.EffectivePersonaState{
				StateID: "eps-test",
				ResolvedTraits: map[string]experience.TraitValueState{
					"warmth": {Value: 0.5},
				},
			},
			Payload: experience.CompiledPersonaPayload{
				PayloadID: "test-payload-id",
			},
		}, nil
	}

	// 1. Initial inspect
	res := doReq(t, srv, http.MethodGet, "/api/experience/inspect", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("inspect status: expected 200, got %d", res.StatusCode)
	}
	defer res.Body.Close()

	var got experienceInspectResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.OwnerID != ownerID {
		t.Fatalf("expected owner id %q, got %q", ownerID, got.OwnerID)
	}
	if got.EffectiveState.StateID != "eps-test" {
		t.Fatalf("expected state id 'eps-test', got %q", got.EffectiveState.StateID)
	}
	if len(got.SnapshotHistory) != 1 {
		t.Fatalf("expected 1 history event (on-demand inspection), got %d", len(got.SnapshotHistory))
	}

	snapshot := got.SnapshotHistory[0]
	if snapshot.Type != schema.FactNaviExperienceSnapshot {
		t.Fatalf("expected snapshot event type, got %s", snapshot.Type)
	}
	payload := snapshot.Payload.(map[string]any)
	if payload["trigger"] != string(experience.SnapshotTriggerOwnerInspect) {
		t.Fatalf("expected owner_inspection trigger, got %v", payload["trigger"])
	}

	// 2. Add some config and inspect again
	_ = experience.SaveCoreIdentityConfiguration(ctx, db, ownerID, map[string]float64{"directness": 0.8}, string(schema.StateKindOwnerSet))

	res2 := doReq(t, srv, http.MethodGet, "/api/experience/inspect", "", "", "127.0.0.1:1234", nil)
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("inspect 2: expected 200, got %d", res2.StatusCode)
	}
	defer res2.Body.Close()

	var got2 experienceInspectResponse
	if err := json.NewDecoder(res2.Body).Decode(&got2); err != nil {
		t.Fatalf("decode 2: %v", err)
	}

	if got2.StoredConfig.CoreIdentity.Traits["directness"] != 0.8 {
		t.Fatalf("expected stored core identity directness=0.8, got %+v", got2.StoredConfig.CoreIdentity)
	}
	// We expect 2 history events: one from the first inspect, and one from the second.
	if len(got2.SnapshotHistory) != 2 {
		t.Fatalf("expected 2 history events, got %d", len(got2.SnapshotHistory))
	}
}
