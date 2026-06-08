package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ProjectReferenceOrphan struct {
	Table     string
	RowID     string
	Field     string
	ProjectID string
	Reason    string
}

type ProjectReferenceAuditReport struct {
	Orphans []ProjectReferenceOrphan
}

func (r ProjectReferenceAuditReport) HasOrphans() bool {
	return len(r.Orphans) > 0
}

func DetectProjectReferenceOrphans(ctx context.Context, db *sql.DB) (ProjectReferenceAuditReport, error) {
	report := ProjectReferenceAuditReport{}
	checks := []struct {
		table    string
		idColumn string
		field    string
	}{
		{table: "artifacts", idColumn: "id", field: "project_id"},
		{table: "tasks", idColumn: "id", field: "project_id"},
		{table: "workspaces", idColumn: "id", field: "related_project_id"},
		{table: "navi_chats", idColumn: "chat_id", field: "project_id"},
		{table: "runtime_sessions", idColumn: "runtime_session_id", field: "project_id"},
	}
	for _, check := range checks {
		rows, err := findProjectReferenceOrphans(ctx, db, check.table, check.idColumn, check.field)
		if err != nil {
			return ProjectReferenceAuditReport{}, err
		}
		report.Orphans = append(report.Orphans, rows...)
	}
	return report, nil
}

func findProjectReferenceOrphans(ctx context.Context, db *sql.DB, table, idColumn, field string) ([]ProjectReferenceOrphan, error) {
	if _, ok, err := lookupTableSQL(ctx, db, table); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	query := fmt.Sprintf(`
		SELECT t.%s, t.%s
		FROM %s t
		LEFT JOIN projects p ON p.id = t.%s
		WHERE t.%s IS NOT NULL AND TRIM(t.%s) <> '' AND p.id IS NULL
		ORDER BY t.%s ASC
	`, quoteIdentifier(idColumn), quoteIdentifier(field), quoteIdentifier(table), quoteIdentifier(field), quoteIdentifier(field), quoteIdentifier(field), quoteIdentifier(idColumn))

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("store: detect orphaned project references in %s.%s: %w", table, field, err)
	}
	defer rows.Close()

	var out []ProjectReferenceOrphan
	for rows.Next() {
		var rowID string
		var projectID string
		if err := rows.Scan(&rowID, &projectID); err != nil {
			return nil, fmt.Errorf("store: scan orphaned project reference in %s.%s: %w", table, field, err)
		}
		projectID = strings.TrimSpace(projectID)
		out = append(out, ProjectReferenceOrphan{
			Table:     table,
			RowID:     rowID,
			Field:     field,
			ProjectID: projectID,
			Reason:    fmt.Sprintf("%s.%s references missing project %q", table, field, projectID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate orphaned project references in %s.%s: %w", table, field, err)
	}
	return out, nil
}
