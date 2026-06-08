package handlers

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	coreskill "github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/store"
)

type SkillEntry = coreskill.SkillEntry
type Interface = coreskill.Interface

var RegisterInternalHandler = coreskill.RegisterInternalHandler

func RegisterSelfDiagnosticHandler(db *sql.DB) {
	if db == nil {
		return
	}
	RegisterInternalHandler("self-diagnostic", "recent_errors", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		limit := 20
		if raw, ok := args["limit"]; ok {
			switch v := raw.(type) {
			case float64:
				limit = int(v)
			case string:
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
					limit = n
				}
			}
		}
		items, err := store.ListErrorRecords(ctx, db, store.ListErrorsFilter{
			Severity:  stringArg(args, "severity"),
			Component: stringArg(args, "component"),
			ChatID:    stringArg(args, "chat_id"),
			ErrorType: stringArg(args, "error_type"),
			Limit:     limit,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"items": items}, nil
	})
	RegisterInternalHandler("self-diagnostic", "error_summary", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		window := stringArg(args, "window")
		if window == "" {
			window = "24h"
		}
		dur, err := time.ParseDuration(window)
		if err != nil {
			return nil, err
		}
		rows, err := store.ErrorSummary(ctx, db, time.Now().UTC().Add(-dur))
		if err != nil {
			return nil, err
		}
		return map[string]any{"window": window, "items": rows}, nil
	})
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
