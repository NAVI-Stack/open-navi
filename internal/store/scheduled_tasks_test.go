package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduledTasks(t *testing.T) {
	ctx := context.Background()
	db := InitTestDB(t)

	now := time.Now().UTC()
	task := schema.ScheduledTask{
		ID:              uuid.New().String(),
		OwnerID:         "test-owner",
		Name:            "Daily Backup",
		Description:     "Run backup every day",
		SchedulePattern: "0 0 * * *",
		Prompt:          "Run the daily backup script now.",
		Status:          "active",
		NextRunAt:       now.Add(-1 * time.Hour), // Due
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Persist
	err := PersistScheduledTask(ctx, db, task)
	require.NoError(t, err)

	// List
	tasks, err := ListScheduledTasks(ctx, db)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, task.ID, tasks[0].ID)

	// GetDue
	due, err := GetDueScheduledTasks(ctx, db, now)
	require.NoError(t, err)
	require.Len(t, due, 1)

	// Not due
	notDue, err := GetDueScheduledTasks(ctx, db, now.Add(-2*time.Hour))
	require.NoError(t, err)
	require.Len(t, notDue, 0)

	// Update
	task.Status = "paused"
	err = UpdateScheduledTask(ctx, db, task)
	require.NoError(t, err)

	due, err = GetDueScheduledTasks(ctx, db, now)
	require.NoError(t, err)
	require.Len(t, due, 0) // because it's paused

	// Delete
	err = DeleteScheduledTask(ctx, db, task.ID)
	require.NoError(t, err)

	tasks, err = ListScheduledTasks(ctx, db)
	require.NoError(t, err)
	require.Len(t, tasks, 0)
}
