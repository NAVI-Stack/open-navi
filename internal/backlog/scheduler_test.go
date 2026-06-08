package backlog

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronScheduling(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

	t.Run("Standard Cron daily", func(t *testing.T) {
		sched, err := parser.Parse("0 9 * * *")
		require.NoError(t, err)

		now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
		next := sched.Next(now)
		assert.Equal(t, 9, next.Hour())
		assert.Equal(t, 0, next.Minute())
		assert.Equal(t, 1, next.Day())

		// After 9 AM
		now = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
		next = sched.Next(now)
		assert.Equal(t, 9, next.Hour())
		assert.Equal(t, 2, next.Day()) // Next day
	})

	t.Run("Interval @every 1h", func(t *testing.T) {
		sched, err := parser.Parse("@every 1h")
		require.NoError(t, err)

		now := time.Date(2026, 1, 1, 8, 30, 0, 0, time.UTC)
		next := sched.Next(now)
		assert.Equal(t, 9, next.Hour())
		assert.Equal(t, 30, next.Minute())
	})

	t.Run("Invalid pattern", func(t *testing.T) {
		_, err := parser.Parse("invalid")
		assert.Error(t, err)
	})
}
