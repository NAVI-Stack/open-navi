package artifact

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
)

type codec interface {
	Parse(raw string) (any, error)
	Serialize(value any) (string, error)
	Validate(value any) error
}

type textCodec struct{}

func (textCodec) Parse(raw string) (any, error) {
	return raw, nil
}

func (textCodec) Serialize(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("artifact: text payload must be a string")
	}
}

func (c textCodec) Validate(value any) error {
	_, err := c.Serialize(value)
	return err
}

type jsonCodec struct{}

func (jsonCodec) Parse(raw string) (any, error) {
	var out any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("artifact: invalid json: %w", err)
	}
	return out, nil
}

func (c jsonCodec) Serialize(value any) (string, error) {
	switch v := value.(type) {
	case string:
		parsed, err := c.Parse(v)
		if err != nil {
			return "", err
		}
		buf, err := json.Marshal(parsed)
		if err != nil {
			return "", fmt.Errorf("artifact: marshal json payload: %w", err)
		}
		return string(buf), nil
	default:
		buf, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("artifact: marshal json payload: %w", err)
		}
		return string(buf), nil
	}
}

func (c jsonCodec) Validate(value any) error {
	_, err := c.Serialize(value)
	return err
}

type tableCodec struct{}

func (tableCodec) Parse(raw string) (any, error) {
	r := csv.NewReader(strings.NewReader(raw))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("artifact: invalid csv: %w", err)
	}
	return records, nil
}

func (c tableCodec) Serialize(value any) (string, error) {
	switch v := value.(type) {
	case string:
		if _, err := c.Parse(v); err != nil {
			return "", err
		}
		return v, nil
	case [][]string:
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		if err := w.WriteAll(v); err != nil {
			return "", fmt.Errorf("artifact: write csv payload: %w", err)
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return "", fmt.Errorf("artifact: write csv payload: %w", err)
		}
		return buf.String(), nil
	default:
		return "", fmt.Errorf("artifact: table payload must be CSV text or [][]string")
	}
}

func (c tableCodec) Validate(value any) error {
	_, err := c.Serialize(value)
	return err
}

func codecForSubtype(subtype string) codec {
	switch normalizeSubtype(subtype) {
	case "json":
		return jsonCodec{}
	case "table", "csv":
		return tableCodec{}
	case "markdown", "text", "summary", "workflow_draft", "code":
		return textCodec{}
	default:
		return textCodec{}
	}
}

func serializeForSubtype(subtype string, payload any) (string, error) {
	return codecForSubtype(subtype).Serialize(payload)
}
