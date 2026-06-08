package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := CreateTables(context.Background(), db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	return db
}

func TestPersistAndGetTask(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	task := schema.NewTask("deploy", "deploy to prod", schema.RiskHigh, schema.AgentNavi)
	task.ProjectID = "proj-1"
	task.WorkspaceID = "ws-1"
	task.TaskClass = schema.TaskClassMutative
	task.RawInput = "deploy to prod"
	task.AcceptanceTarget = "deployment validated"
	task.LifecyclePhase = "intake_accepted"
	if err := PersistTask(ctx, db, task); err != nil {
		t.Fatalf("PersistTask: %v", err)
	}
	got, ok, err := GetTaskByID(ctx, db, task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if !ok {
		t.Fatal("expected task to be found")
	}
	if got.ID != task.ID || got.Title != task.Title {
		t.Fatalf("got %+v", got)
	}
	if got.ProjectID != "proj-1" || got.WorkspaceID != "ws-1" || got.TaskClass != schema.TaskClassMutative {
		t.Fatalf("project task metadata did not round-trip: %+v", got)
	}
	if got.RawInput != "deploy to prod" || got.AcceptanceTarget != "deployment validated" || got.LifecyclePhase != "intake_accepted" {
		t.Fatalf("project task intake fields did not round-trip: %+v", got)
	}
}

func TestGetTaskByIDNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	_, ok, err := GetTaskByID(ctx, db, "nonexistent")
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if ok {
		t.Fatal("expected not found")
	}
}

func TestUpdateTask(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	task := schema.NewTask("deploy", "deploy to prod", schema.RiskHigh, schema.AgentNavi)
	_ = PersistTask(ctx, db, task)
	task.Status = schema.TaskStatusRunning
	task.Title = "deploy v2"
	if err := UpdateTask(ctx, db, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got, ok, _ := GetTaskByID(ctx, db, task.ID)
	if !ok || got.Status != schema.TaskStatusRunning || got.Title != "deploy v2" {
		t.Fatalf("got %+v", got)
	}
}

func TestListTasksByStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t1 := schema.NewTask("a", "d", schema.RiskLow, schema.AgentConnector)
	t2 := schema.NewTask("b", "d", schema.RiskLow, schema.AgentConnector)
	t2.Status = schema.TaskStatusCompleted
	_ = PersistTask(ctx, db, t1)
	_ = PersistTask(ctx, db, t2)
	list, err := ListTasksByStatus(ctx, db, schema.TaskStatusPending)
	if err != nil {
		t.Fatalf("ListTasksByStatus: %v", err)
	}
	if len(list) != 1 || list[0].ID != t1.ID {
		t.Fatalf("expected one pending, got %d", len(list))
	}
}

func TestListTasksByAgent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	task := schema.NewTask("x", "d", schema.RiskMedium, schema.AgentConnector)
	_ = PersistTask(ctx, db, task)
	list, err := ListTasksByAgent(ctx, db, schema.AgentConnector)
	if err != nil {
		t.Fatalf("ListTasksByAgent: %v", err)
	}
	if len(list) != 1 || list[0].AssignedTo != schema.AgentConnector {
		t.Fatalf("got %+v", list)
	}
}

func TestListTasksByRisk(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	task := schema.NewTask("y", "d", schema.RiskCritical, schema.AgentHeartbeat)
	_ = PersistTask(ctx, db, task)
	list, err := ListTasksByRisk(ctx, db, schema.RiskCritical)
	if err != nil {
		t.Fatalf("ListTasksByRisk: %v", err)
	}
	if len(list) != 1 || list[0].Risk != schema.RiskCritical {
		t.Fatalf("got %+v", list)
	}
}

func TestListAndCancelProjectTasks(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	task := schema.NewTask("project task", "do project work", schema.RiskLow, schema.AgentNavi)
	task.ProjectID = "proj-1"
	task.TaskClass = schema.TaskClassPlanning
	task.RawInput = "do project work"
	task.AcceptanceTarget = "done"
	task.LifecyclePhase = "intake_accepted"
	if err := PersistTask(ctx, db, task); err != nil {
		t.Fatalf("PersistTask: %v", err)
	}
	other := schema.NewTask("other", "other", schema.RiskLow, schema.AgentNavi)
	other.ProjectID = "proj-2"
	other.TaskClass = schema.TaskClassPlanning
	other.RawInput = "other"
	if err := PersistTask(ctx, db, other); err != nil {
		t.Fatalf("PersistTask(other): %v", err)
	}

	list, err := ListTasksByProject(ctx, db, "proj-1")
	if err != nil {
		t.Fatalf("ListTasksByProject: %v", err)
	}
	if len(list) != 1 || list[0].ID != task.ID {
		t.Fatalf("expected one project task, got %+v", list)
	}
	cancelled, err := CancelProjectTask(ctx, db, "proj-1", task.ID, task.UpdatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("CancelProjectTask: %v", err)
	}
	if cancelled.Status != schema.TaskStatusCancelled || cancelled.LifecyclePhase != "cancelled" {
		t.Fatalf("expected cancelled task, got %+v", cancelled)
	}
}
