package artifact

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type diffPayload struct {
	Subtype string `json:"subtype"`
	Kind    string `json:"kind"`
	Changed bool   `json:"changed"`
	Before  any    `json:"before,omitempty"`
	After   any    `json:"after,omitempty"`
}

func buildDiffPayload(subtype, before, after string) (string, error) {
	switch normalizeSubtype(subtype) {
	case "json":
		return buildJSONDiff(subtype, before, after)
	case "table", "csv":
		return buildTableDiff(subtype, before, after)
	default:
		return buildTextDiff(subtype, before, after)
	}
}

func buildTextDiff(subtype, before, after string) (string, error) {
	payload := diffPayload{
		Subtype: subtype,
		Kind:    "line",
		Changed: before != after,
		Before:  strings.Split(before, "\n"),
		After:   strings.Split(after, "\n"),
	}
	return marshalDiffPayload(payload)
}

func buildJSONDiff(subtype, before, after string) (string, error) {
	beforeParsed, err := codecForSubtype(subtype).Parse(before)
	if err != nil {
		return "", err
	}
	afterParsed, err := codecForSubtype(subtype).Parse(after)
	if err != nil {
		return "", err
	}
	payload := diffPayload{
		Subtype: subtype,
		Kind:    "structured",
		Changed: !reflect.DeepEqual(beforeParsed, afterParsed),
		Before:  beforeParsed,
		After:   afterParsed,
	}
	return marshalDiffPayload(payload)
}

func buildTableDiff(subtype, before, after string) (string, error) {
	beforeParsed, err := codecForSubtype(subtype).Parse(before)
	if err != nil {
		return "", err
	}
	afterParsed, err := codecForSubtype(subtype).Parse(after)
	if err != nil {
		return "", err
	}
	payload := diffPayload{
		Subtype: subtype,
		Kind:    "table",
		Changed: !reflect.DeepEqual(beforeParsed, afterParsed),
		Before:  beforeParsed,
		After:   afterParsed,
	}
	return marshalDiffPayload(payload)
}

func marshalDiffPayload(payload diffPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("artifact: marshal diff payload: %w", err)
	}
	return string(raw), nil
}
