package store

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/schema"
)

func TestRegisterAndGetAgent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if err := RegisterAgent(ctx, db, "agent-dev-1", schema.AgentNavi); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	typ, live, err := GetAgent(ctx, db, "agent-dev-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if typ != schema.AgentNavi || !live {
		t.Fatalf("got type=%q live=%v", typ, live)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	_, live, err := GetAgent(ctx, db, "nonexistent")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if live {
		t.Fatal("expected not live when not found")
	}
}

func TestRetireAndListLive(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	_ = RegisterAgent(ctx, db, "agent-1", schema.AgentConnector)
	_ = RegisterAgent(ctx, db, "agent-2", schema.AgentNavi)
	if err := RetireAgent(ctx, db, "agent-1"); err != nil {
		t.Fatalf("RetireAgent: %v", err)
	}
	list, err := ListLiveAgents(ctx, db)
	if err != nil {
		t.Fatalf("ListLiveAgents: %v", err)
	}
	if len(list) != 1 || list[0].AgentID != "agent-2" {
		t.Fatalf("expected one live agent, got %+v", list)
	}
	_, live, _ := GetAgent(ctx, db, "agent-1")
	if live {
		t.Fatal("agent-1 should be retired")
	}
}
