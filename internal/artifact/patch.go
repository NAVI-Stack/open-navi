package artifact

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

func prepareContentForOperation(subtype string, op schema.ArtifactOperation, baseContent string, payload any) (string, error) {
	switch op {
	case schema.ArtifactOpAppend:
		return applyAppend(subtype, baseContent, payload)
	case schema.ArtifactOpPatch:
		return applyPatch(subtype, baseContent, payload)
	default:
		return serializePayload(subtype, payload)
	}
}

func applyAppend(subtype, baseContent string, payload any) (string, error) {
	switch normalizeSubtype(subtype) {
	case "json":
		baseParsed, err := codecForSubtype(subtype).Parse(baseContent)
		if err != nil {
			return "", err
		}
		values, ok := baseParsed.([]any)
		if !ok {
			return "", fmt.Errorf("artifact: json append requires array content")
		}
		values = append(values, payload)
		return serializeForSubtype(subtype, values)
	case "table", "csv":
		baseParsed, err := codecForSubtype(subtype).Parse(baseContent)
		if err != nil {
			return "", err
		}
		rows, ok := baseParsed.([][]string)
		if !ok {
			return "", fmt.Errorf("artifact: table append requires CSV content")
		}
		switch typed := payload.(type) {
		case []string:
			rows = append(rows, typed)
		case [][]string:
			rows = append(rows, typed...)
		case string:
			parsed, err := codecForSubtype(subtype).Parse(typed)
			if err != nil {
				return "", err
			}
			appendRows, ok := parsed.([][]string)
			if !ok {
				return "", fmt.Errorf("artifact: table append requires rows")
			}
			rows = append(rows, appendRows...)
		default:
			return "", fmt.Errorf("artifact: unsupported table append payload %T", payload)
		}
		return serializeForSubtype(subtype, rows)
	default:
		addition, err := serializeForSubtype(subtype, payload)
		if err != nil {
			return "", err
		}
		return baseContent + addition, nil
	}
}

func applyPatch(subtype, baseContent string, payload any) (string, error) {
	switch normalizeSubtype(subtype) {
	case "json":
		return applyJSONPatch(subtype, baseContent, payload)
	case "table", "csv":
		return applyTablePatch(subtype, baseContent, payload)
	default:
		return applyTextPatch(subtype, baseContent, payload)
	}
}

func applyTextPatch(subtype, baseContent string, payload any) (string, error) {
	if replacement, ok := scalarReplacementPayload(payload); ok {
		return serializeForSubtype(subtype, replacement)
	}
	patch, ok := payload.(map[string]any)
	if !ok {
		return "", fmt.Errorf("artifact: text patch requires string or object payload")
	}
	mode := strings.ToLower(strings.TrimSpace(asString(patch["mode"])))
	switch mode {
	case "", "replace", "full":
		replacement, ok := extractReplacementPayload(patch)
		if !ok {
			return "", fmt.Errorf("artifact: replacement patch missing content")
		}
		return serializeForSubtype(subtype, replacement)
	case "text_range":
		start, end, err := patchRange(patch, len(baseContent))
		if err != nil {
			return "", err
		}
		insert := firstString(patch["text"], patch["value"], patch["content"])
		return baseContent[:start] + insert + baseContent[end:], nil
	case "block", "function":
		target := firstString(patch["target"], patch["find"], patch["old"])
		if strings.TrimSpace(target) == "" {
			return "", fmt.Errorf("artifact: block patch requires target text")
		}
		replacement := firstString(patch["text"], patch["value"], patch["content"])
		if !strings.Contains(baseContent, target) {
			return "", fmt.Errorf("artifact: block patch target not found")
		}
		return strings.Replace(baseContent, target, replacement, 1), nil
	default:
		return "", fmt.Errorf("artifact: unsupported text patch mode %q", mode)
	}
}

func applyJSONPatch(subtype, baseContent string, payload any) (string, error) {
	if replacement, ok := scalarReplacementPayload(payload); ok {
		return serializeForSubtype(subtype, replacement)
	}
	patch, ok := payload.(map[string]any)
	if !ok {
		return "", fmt.Errorf("artifact: json patch requires object payload")
	}
	baseParsed, err := codecForSubtype(subtype).Parse(baseContent)
	if err != nil {
		return "", err
	}
	mode := strings.ToLower(strings.TrimSpace(asString(patch["mode"])))
	switch mode {
	case "merge", "structured":
		baseMap, ok := baseParsed.(map[string]any)
		if !ok {
			return "", fmt.Errorf("artifact: structured json patch requires object content")
		}
		value := patch["value"]
		if value == nil {
			value = patch["patch"]
		}
		mergeMap, ok := coerceMap(value)
		if !ok {
			return "", fmt.Errorf("artifact: structured json patch requires object value")
		}
		for key, raw := range mergeMap {
			baseMap[key] = raw
		}
		return serializeForSubtype(subtype, baseMap)
	case "":
		baseMap, ok := baseParsed.(map[string]any)
		if !ok {
			return "", fmt.Errorf("artifact: structured json patch requires object content")
		}
		if replacement, ok := extractReplacementPayload(patch); ok {
			return serializeForSubtype(subtype, replacement)
		}
		mergeMap := patchMetadataBody(patch)
		for key, raw := range mergeMap {
			baseMap[key] = raw
		}
		return serializeForSubtype(subtype, baseMap)
	case "row_cell":
		rows, ok := baseParsed.([]any)
		if !ok {
			return "", fmt.Errorf("artifact: row/cell patch requires array content")
		}
		rowIndex, ok := asInt(patch["row"])
		if !ok || rowIndex < 0 || rowIndex >= len(rows) {
			return "", fmt.Errorf("artifact: row/cell patch row out of range")
		}
		rowMap, ok := rows[rowIndex].(map[string]any)
		if !ok {
			return "", fmt.Errorf("artifact: row/cell patch requires array of objects")
		}
		column := firstString(patch["column"], patch["field"])
		if strings.TrimSpace(column) == "" {
			return "", fmt.Errorf("artifact: row/cell patch requires column")
		}
		rowMap[column] = patch["value"]
		return serializeForSubtype(subtype, rows)
	case "replace", "full":
		replacement, ok := extractReplacementPayload(patch)
		if !ok {
			return "", fmt.Errorf("artifact: replacement patch missing content")
		}
		return serializeForSubtype(subtype, replacement)
	default:
		return "", fmt.Errorf("artifact: unsupported json patch mode %q", mode)
	}
}

func applyTablePatch(subtype, baseContent string, payload any) (string, error) {
	if replacement, ok := scalarReplacementPayload(payload); ok {
		return serializeForSubtype(subtype, replacement)
	}
	patch, ok := payload.(map[string]any)
	if !ok {
		return "", fmt.Errorf("artifact: table patch requires object payload")
	}
	baseParsed, err := codecForSubtype(subtype).Parse(baseContent)
	if err != nil {
		return "", err
	}
	rows, ok := baseParsed.([][]string)
	if !ok || len(rows) == 0 {
		return "", fmt.Errorf("artifact: table patch requires table rows")
	}
	mode := strings.ToLower(strings.TrimSpace(asString(patch["mode"])))
	switch mode {
	case "row_cell", "structured":
		rowIndex, ok := asInt(patch["row"])
		if !ok || rowIndex < 0 || rowIndex >= len(rows)-1 {
			return "", fmt.Errorf("artifact: row/cell patch row out of range")
		}
		columnIndex, err := resolveColumnIndex(rows[0], patch["column"])
		if err != nil {
			return "", err
		}
		rows[rowIndex+1][columnIndex] = firstString(patch["value"], patch["text"], patch["content"])
		return serializeCSVRows(rows)
	case "", "replace", "full":
		replacement, ok := extractReplacementPayload(patch)
		if !ok {
			return "", fmt.Errorf("artifact: replacement patch missing content")
		}
		return serializeForSubtype(subtype, replacement)
	default:
		return "", fmt.Errorf("artifact: unsupported table patch mode %q", mode)
	}
}

func patchRange(patch map[string]any, max int) (int, int, error) {
	start, ok := asInt(patch["start"])
	if !ok {
		return 0, 0, fmt.Errorf("artifact: text_range patch requires start")
	}
	end, ok := asInt(patch["end"])
	if !ok {
		return 0, 0, fmt.Errorf("artifact: text_range patch requires end")
	}
	if start < 0 || end < start || end > max {
		return 0, 0, fmt.Errorf("artifact: invalid text range [%d,%d)", start, end)
	}
	return start, end, nil
}

func resolveColumnIndex(header []string, raw any) (int, error) {
	if idx, ok := asInt(raw); ok {
		if idx < 0 || idx >= len(header) {
			return 0, fmt.Errorf("artifact: column index out of range")
		}
		return idx, nil
	}
	name := strings.TrimSpace(asString(raw))
	if name == "" {
		return 0, fmt.Errorf("artifact: row/cell patch requires column")
	}
	for i, candidate := range header {
		if candidate == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("artifact: unknown column %q", name)
}

func serializeCSVRows(rows [][]string) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(rows); err != nil {
		return "", fmt.Errorf("artifact: write csv payload: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("artifact: write csv payload: %w", err)
	}
	return buf.String(), nil
}

func extractReplacementPayload(payload any) (any, bool) {
	switch typed := payload.(type) {
	case string, []byte, [][]string:
		return typed, true
	case map[string]any:
		for _, key := range []string{"content", "text", "value", "rows"} {
			if raw, ok := typed[key]; ok {
				switch key {
				case "value":
					if _, isScalar := raw.(map[string]any); isScalar || raw != nil {
						return raw, true
					}
				default:
					return raw, true
				}
			}
		}
	}
	return nil, false
}

func scalarReplacementPayload(payload any) (any, bool) {
	switch typed := payload.(type) {
	case string, []byte, [][]string:
		return typed, true
	default:
		return nil, false
	}
}

func patchMetadataBody(patch map[string]any) map[string]any {
	out := make(map[string]any, len(patch))
	for key, value := range patch {
		switch key {
		case "mode", "start", "end", "row", "column", "field", "target", "find", "old", "text", "content", "rows":
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func coerceMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case string:
		var out map[string]any
		if err := json.Unmarshal([]byte(typed), &out); err != nil {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

func firstString(values ...any) string {
	for _, value := range values {
		if raw := asString(value); strings.TrimSpace(raw) != "" {
			return raw
		}
	}
	return ""
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		i, err := typed.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(typed))
		return i, err == nil
	default:
		return 0, false
	}
}
