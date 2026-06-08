package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// ExportRequest describes an artifact export request.
type ExportRequest struct {
	ArtifactID       string
	VersionID        string
	Format           schema.ExportFormat
	TargetKind       schema.ArtifactExportTargetKind
	TargetURI        string
	ActorType        schema.ActorType
	ActorID          string
	ConfirmationMode string
}

// ShareRequest describes an artifact share request.
type ShareRequest struct {
	ArtifactID       string
	VersionID        string
	Scope            schema.ArtifactShareScope
	AccessLevel      schema.ArtifactShareAccessLevel
	ActorType        schema.ActorType
	ActorID          string
	ConfirmationMode string
	ExpiresAt        *time.Time
}

// ExportArtifact generates a durable export record and payload without mutating artifact content.
func (s *Service) ExportArtifact(ctx context.Context, req ExportRequest) (schema.ArtifactExport, error) {
	startedAt := time.Now().UTC()
	artifact, version, err := s.resolveArtifactVersion(ctx, req.ArtifactID, req.VersionID)
	if err != nil {
		return schema.ArtifactExport{}, err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionStarted, artifact, &version, "export_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "started", "", "", startedAt, req.ActorType, req.ActorID)
	export := schema.ArtifactExport{
		ExportID:           uuid.New().String(),
		ArtifactID:         artifact.ID,
		VersionID:          version.ID,
		Format:             req.Format,
		TargetKind:         req.TargetKind,
		TargetURI:          strings.TrimSpace(req.TargetURI),
		Status:             schema.ArtifactExportPending,
		CreatedByActorType: firstActorType(req.ActorType),
		CreatedByActorID:   req.ActorID,
		CreatedAt:          time.Now().UTC(),
	}
	if err := store.SaveArtifactExport(ctx, s.db, export); err != nil {
		return schema.ArtifactExport{}, err
	}

	if err := requireExportGovernance(req.TargetKind, req.ConfirmationMode); err != nil {
		return s.failExport(ctx, artifact, export, schema.FailureClassPolicyBlocked, err)
	}
	content, err := s.ReadVersionContent(ctx, version.ID)
	if err != nil {
		return s.failExport(ctx, artifact, export, schema.FailureClassStorageFailure, fmt.Errorf("artifact: read export source: %w", err))
	}
	desc := s.registry.GetRenderer(artifact.Subtype)
	if !supportsExportFormat(desc.ExportFormats, req.Format) && !supportsExportFormat(s.registry.GetEditor(artifact.Subtype).ExportFormats, req.Format) {
		return s.failExport(ctx, artifact, export, schema.FailureClassRendererMissing, fmt.Errorf("artifact: export format %s unsupported for subtype %s", req.Format, artifact.Subtype))
	}

	payload, contentType, ext, err := buildExportPayload(artifact.Subtype, req.Format, content)
	if err != nil {
		class := schema.FailureClassExportFailure
		if strings.Contains(err.Error(), "unsupported") {
			class = schema.FailureClassRendererMissing
		}
		return s.failExport(ctx, artifact, export, class, err)
	}
	key := fmt.Sprintf("artifacts/%s/exports/%s.%s", artifact.ID, export.ExportID, ext)
	if err := s.blob.Put(ctx, key, bytes.NewReader(payload)); err != nil {
		return s.failExport(ctx, artifact, export, schema.FailureClassStorageFailure, fmt.Errorf("artifact: store export payload: %w", err))
	}

	sum := sha256.Sum256(payload)
	completedAt := time.Now().UTC()
	export.ContentRef = &schema.ContentRef{
		URI:            fmt.Sprintf("artifact://artifacts/%s/exports/%s/content", artifact.ID, export.ExportID),
		StorageKey:     key,
		ChecksumSHA256: hex.EncodeToString(sum[:]),
	}
	export.ContentType = contentType
	export.Status = schema.ArtifactExportCompleted
	export.CompletedAt = &completedAt
	if err := store.SaveArtifactExport(ctx, s.db, export); err != nil {
		return schema.ArtifactExport{}, err
	}

	if artifact.Attributes == nil {
		artifact.Attributes = map[string]any{}
	}
	artifact.Attributes["exported"] = true
	artifact.Attributes["exported_at"] = completedAt.Format(time.RFC3339)
	artifact.Attributes["sync_state"] = "exported"
	artifact.Attributes["last_export_id"] = export.ExportID
	artifact.Attributes["last_export_format"] = string(export.Format)
	artifact.Attributes["last_export_target_kind"] = string(export.TargetKind)
	_ = clearArtifactFailureState(artifact)
	if err := store.SaveArtifact(ctx, s.db, *artifact); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, artifact, &version, "export_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, req.ActorType, req.ActorID)
		return schema.ArtifactExport{}, err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactExportCompleted, artifact, &version, "export_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "completed", "", "", startedAt, req.ActorType, req.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, artifact, &version, "export_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "completed", "", "", startedAt, req.ActorType, req.ActorID)
	s.emitArtifactReflection(ctx, artifact, &version, "export_artifact", fmt.Sprintf("Artifact %s exported", firstNonEmpty(artifact.DisplayTitle, artifact.CanonicalTitle, artifact.ID)), "", s.registry.GetRenderer(artifact.Subtype).ComponentID, "completed", "", "")
	return export, nil
}

// ShareArtifact creates a durable sharing record and updates artifact share metadata.
func (s *Service) ShareArtifact(ctx context.Context, req ShareRequest) (schema.ArtifactShare, error) {
	startedAt := time.Now().UTC()
	artifact, version, err := s.resolveArtifactVersion(ctx, req.ArtifactID, req.VersionID)
	if err != nil {
		return schema.ArtifactShare{}, err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionStarted, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "started", "", "", startedAt, req.ActorType, req.ActorID)
	if err := requireShareGovernance(req.Scope, req.ConfirmationMode); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactSyncFailed, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassPolicyBlocked, "share_governance", startedAt, req.ActorType, req.ActorID)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassPolicyBlocked, "share_governance", startedAt, req.ActorType, req.ActorID)
		s.emitArtifactReflection(ctx, artifact, &version, "share_artifact", fmt.Sprintf("Artifact %s share blocked", firstNonEmpty(artifact.DisplayTitle, artifact.CanonicalTitle, artifact.ID)), "", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassPolicyBlocked, "share_governance")
		return schema.ArtifactShare{}, err
	}
	share := schema.ArtifactShare{
		ShareID:            uuid.New().String(),
		ArtifactID:         artifact.ID,
		VersionID:          version.ID,
		Scope:              req.Scope,
		AccessLevel:        req.AccessLevel,
		Status:             schema.ArtifactShareStatusActive,
		CreatedByActorType: firstActorType(req.ActorType),
		CreatedByActorID:   req.ActorID,
		CreatedAt:          time.Now().UTC(),
		ExpiresAt:          req.ExpiresAt,
	}
	if share.Scope == schema.ArtifactShareScopeLink {
		share.ShareURL = fmt.Sprintf("artifact://share/%s", share.ShareID)
	}
	if err := store.SaveArtifactShare(ctx, s.db, share); err != nil {
		return schema.ArtifactShare{}, err
	}
	if artifact.Attributes == nil {
		artifact.Attributes = map[string]any{}
	}
	artifact.Attributes["shared"] = true
	artifact.Attributes["shared_at"] = share.CreatedAt.Format(time.RFC3339)
	artifact.Attributes["share_state"] = "shared"
	artifact.Attributes["last_share_id"] = share.ShareID
	if share.ShareURL != "" {
		artifact.Attributes["share_url"] = share.ShareURL
	}
	if err := store.SaveArtifact(ctx, s.db, *artifact); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactSyncFailed, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, req.ActorType, req.ActorID)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, req.ActorType, req.ActorID)
		s.emitArtifactReflection(ctx, artifact, &version, "share_artifact", fmt.Sprintf("Artifact %s share failed", firstNonEmpty(artifact.DisplayTitle, artifact.CanonicalTitle, artifact.ID)), "", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact")
		return schema.ArtifactShare{}, err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactSyncCompleted, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "completed", "", "", startedAt, req.ActorType, req.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, artifact, &version, "share_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "completed", "", "", startedAt, req.ActorType, req.ActorID)
	s.emitArtifactReflection(ctx, artifact, &version, "share_artifact", fmt.Sprintf("Artifact %s shared", firstNonEmpty(artifact.DisplayTitle, artifact.CanonicalTitle, artifact.ID)), "", s.registry.GetRenderer(artifact.Subtype).ComponentID, "completed", "", "")
	return share, nil
}

// ReadExportContent loads a generated export payload.
func (s *Service) ReadExportContent(ctx context.Context, exportID string) ([]byte, schema.ArtifactExport, error) {
	export, err := store.GetArtifactExport(ctx, s.db, exportID)
	if err != nil {
		return nil, schema.ArtifactExport{}, err
	}
	if export == nil {
		return nil, schema.ArtifactExport{}, fmt.Errorf("artifact: export not found: %s", exportID)
	}
	if export.ContentRef == nil || strings.TrimSpace(export.ContentRef.StorageKey) == "" {
		return nil, schema.ArtifactExport{}, fmt.Errorf("artifact: export content unavailable: %s", exportID)
	}
	reader, err := s.blob.Get(ctx, export.ContentRef.StorageKey)
	if err != nil {
		return nil, schema.ArtifactExport{}, err
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, schema.ArtifactExport{}, fmt.Errorf("artifact: read export payload: %w", err)
	}
	return payload, *export, nil
}

func (s *Service) resolveArtifactVersion(ctx context.Context, artifactID, versionID string) (*schema.Artifact, schema.ArtifactVersion, error) {
	artifact, err := store.GetArtifact(ctx, s.db, artifactID)
	if err != nil {
		return nil, schema.ArtifactVersion{}, err
	}
	if artifact == nil {
		return nil, schema.ArtifactVersion{}, fmt.Errorf("artifact: artifact not found: %s", artifactID)
	}
	if strings.TrimSpace(versionID) == "" {
		versionID = artifact.CurrentVersionID
	}
	if strings.TrimSpace(versionID) == "" {
		return nil, schema.ArtifactVersion{}, fmt.Errorf("artifact: artifact version not found: %s", artifactID)
	}
	version, err := store.GetArtifactVersion(ctx, s.db, versionID)
	if err != nil {
		return nil, schema.ArtifactVersion{}, err
	}
	if version.ArtifactID != artifact.ID {
		return nil, schema.ArtifactVersion{}, fmt.Errorf("artifact: version %s does not belong to artifact %s", versionID, artifact.ID)
	}
	return artifact, version, nil
}

func (s *Service) failExport(ctx context.Context, artifact *schema.Artifact, export schema.ArtifactExport, failureClass schema.FailureClass, err error) (schema.ArtifactExport, error) {
	export.Status = schema.ArtifactExportFailed
	export.FailureClass = failureClass
	export.FailureReason = err.Error()
	completedAt := time.Now().UTC()
	export.CompletedAt = &completedAt
	_ = store.SaveArtifactExport(ctx, s.db, export)
	if artifact != nil {
		if artifact.Attributes == nil {
			artifact.Attributes = map[string]any{}
		}
		recovery, _ := json.Marshal(map[string]any{
			"failed_export_id": export.ExportID,
			"retryable":        true,
		})
		artifact.LastError = err.Error()
		artifact.RecoverableOutput = string(recovery)
		artifact.Attributes["last_error"] = err.Error()
		artifact.Attributes["failure_class"] = string(failureClass)
		artifact.Attributes["recoverable_output"] = decodeFlexibleRecovery(recovery)
		_ = store.SaveArtifact(ctx, s.db, *artifact)
		if failureClass == schema.FailureClassRendererMissing {
			s.emitArtifactEvent(ctx, schema.FactArtifactRendererMissing, artifact, nil, "export_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "missing", failureClass, "export_failed", time.Now().UTC(), export.CreatedByActorType, export.CreatedByActorID)
		}
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, artifact, nil, "export_artifact", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", failureClass, "export_failed", time.Now().UTC(), export.CreatedByActorType, export.CreatedByActorID)
		s.emitArtifactReflection(ctx, artifact, nil, "export_artifact", fmt.Sprintf("Artifact %s export failed", firstNonEmpty(artifact.DisplayTitle, artifact.CanonicalTitle, artifact.ID)), "", s.registry.GetRenderer(artifact.Subtype).ComponentID, "failed", failureClass, "export_failed")
	}
	return export, err
}

func buildExportPayload(subtype string, format schema.ExportFormat, content string) ([]byte, string, string, error) {
	switch format {
	case schema.ExportFormatMarkdown:
		return []byte(content), "text/markdown; charset=utf-8", "md", nil
	case schema.ExportFormatText, schema.ExportFormatFormatted:
		return []byte(content), "text/plain; charset=utf-8", "txt", nil
	case schema.ExportFormatJSON:
		return buildJSONExport(subtype, content)
	case schema.ExportFormatCSV:
		return buildCSVExport(subtype, content)
	case schema.ExportFormatPDF:
		return buildPDFExport(content)
	default:
		return nil, "", "", fmt.Errorf("artifact: unsupported export format %s", format)
	}
}

func buildJSONExport(subtype, content string) ([]byte, string, string, error) {
	switch normalizeSubtype(subtype) {
	case "json":
		parsed, err := codecForSubtype(subtype).Parse(content)
		if err != nil {
			return nil, "", "", err
		}
		buf, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			return nil, "", "", fmt.Errorf("artifact: encode json export: %w", err)
		}
		return buf, "application/json", "json", nil
	case "table", "csv":
		parsed, err := codecForSubtype(subtype).Parse(content)
		if err != nil {
			return nil, "", "", err
		}
		rows, ok := parsed.([][]string)
		if !ok {
			return nil, "", "", fmt.Errorf("artifact: invalid table export payload")
		}
		records := csvRowsToObjects(rows)
		buf, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return nil, "", "", fmt.Errorf("artifact: encode table json export: %w", err)
		}
		return buf, "application/json", "json", nil
	default:
		return nil, "", "", fmt.Errorf("artifact: unsupported json export for subtype %s", subtype)
	}
}

func buildCSVExport(subtype, content string) ([]byte, string, string, error) {
	switch normalizeSubtype(subtype) {
	case "table", "csv":
		return []byte(content), "text/csv; charset=utf-8", "csv", nil
	case "json":
		parsed, err := codecForSubtype(subtype).Parse(content)
		if err != nil {
			return nil, "", "", err
		}
		rows, err := jsonObjectsToCSV(parsed)
		if err != nil {
			return nil, "", "", err
		}
		return []byte(rows), "text/csv; charset=utf-8", "csv", nil
	default:
		return nil, "", "", fmt.Errorf("artifact: unsupported csv export for subtype %s", subtype)
	}
}

func buildPDFExport(content string) ([]byte, string, string, error) {
	safe := escapePDFText(content)
	stream := fmt.Sprintf("BT /F1 12 Tf 50 780 Td (%s) Tj ET", safe)
	pdf := fmt.Sprintf("%%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Count 1 /Kids [3 0 R] >> endobj\n3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >> endobj\n4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n5 0 obj << /Length %d >> stream\n%s\nendstream endobj\nxref\n0 6\n0000000000 65535 f \ntrailer << /Root 1 0 R /Size 6 >>\nstartxref\n0\n%%%%EOF", len(stream), stream)
	return []byte(pdf), "application/pdf", "pdf", nil
}

func escapePDFText(content string) string {
	content = strings.ReplaceAll(content, "\\", "\\\\")
	content = strings.ReplaceAll(content, "(", "\\(")
	content = strings.ReplaceAll(content, ")", "\\)")
	lines := strings.Split(content, "\n")
	if len(lines) > 8 {
		lines = lines[:8]
	}
	return strings.Join(lines, "\\n")
}

func csvRowsToObjects(rows [][]string) []map[string]string {
	if len(rows) == 0 {
		return nil
	}
	headers := rows[0]
	out := make([]map[string]string, 0, maxInt(len(rows)-1, 0))
	for _, row := range rows[1:] {
		item := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(row) {
				item[header] = row[i]
			} else {
				item[header] = ""
			}
		}
		out = append(out, item)
	}
	return out
}

func jsonObjectsToCSV(value any) (string, error) {
	items, ok := value.([]any)
	if !ok {
		return "", fmt.Errorf("artifact: json csv export requires an array of objects")
	}
	if len(items) == 0 {
		return "", nil
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("artifact: json csv export requires an array of objects")
	}
	headers := make([]string, 0, len(first))
	for key := range first {
		headers = append(headers, key)
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(headers); err != nil {
		return "", fmt.Errorf("artifact: write csv header: %w", err)
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return "", fmt.Errorf("artifact: json csv export requires an array of objects")
		}
		row := make([]string, 0, len(headers))
		for _, header := range headers {
			row = append(row, fmt.Sprint(obj[header]))
		}
		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("artifact: write csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("artifact: flush csv export: %w", err)
	}
	return buf.String(), nil
}

func supportsExportFormat(formats []string, format schema.ExportFormat) bool {
	for _, item := range formats {
		if strings.EqualFold(strings.TrimSpace(item), string(format)) {
			return true
		}
	}
	return false
}

func requireExportGovernance(targetKind schema.ArtifactExportTargetKind, confirmationMode string) error {
	if targetKind == schema.ArtifactExportTargetExternalSystem && strings.TrimSpace(confirmationMode) == "" {
		return fmt.Errorf("artifact: export to external_system requires confirmation")
	}
	return nil
}

func requireShareGovernance(scope schema.ArtifactShareScope, confirmationMode string) error {
	if (scope == schema.ArtifactShareScopeOrg || scope == schema.ArtifactShareScopeLink) && strings.TrimSpace(confirmationMode) == "" {
		return fmt.Errorf("artifact: share scope %s requires confirmation", scope)
	}
	return nil
}

func firstActorType(actorType schema.ActorType) schema.ActorType {
	if actorType == "" {
		return schema.ActorAgent
	}
	return actorType
}

func clearArtifactFailureState(artifact *schema.Artifact) error {
	if artifact == nil {
		return nil
	}
	artifact.LastError = ""
	artifact.RecoverableOutput = ""
	if artifact.Attributes != nil {
		delete(artifact.Attributes, "last_error")
		delete(artifact.Attributes, "failure_class")
		delete(artifact.Attributes, "recoverable_output")
	}
	return nil
}

func decodeFlexibleRecovery(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ExportFilename returns a stable download filename for an export.
func ExportFilename(title string, format schema.ExportFormat) string {
	base := strings.ToLower(strings.TrimSpace(title))
	if base == "" {
		base = "artifact"
	}
	base = strings.ReplaceAll(base, " ", "-")
	base = url.PathEscape(base)
	return base + "." + string(format)
}
