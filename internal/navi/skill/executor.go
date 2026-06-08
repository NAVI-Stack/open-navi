package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotExecutable = errors.New("skill transport not executable")

// Execute runs a skill interface and returns a transport-normalized JSON result.
// ErrNotExecutable is reserved for transports that are declared but not yet
// supported in this deployment.
func Execute(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (string, error) {
	if entry == nil || iface == nil || entry.Spec == nil {
		return "", ErrNotExecutable
	}
	if args == nil {
		args = map[string]any{}
	}

	switch iface.Transport.Type {
	case "subprocess":
		return executeSubprocess(ctx, entry, iface, args)
	case "subprocess_python":
		return executeSubprocessPython(ctx, entry, iface, args)
	case "rest":
		return executeREST(ctx, entry, iface, args)
	case "internal":
		return executeInternal(ctx, entry, iface, args)
	case "mcp_tool":
		return executeMCPTool(ctx, entry, iface, args)
	default:
		return "", fmt.Errorf("unsupported transport type %q", iface.Transport.Type)
	}
}

// executeMCPTool invokes an MCP tool via an HTTP bridge that speaks MCP on behalf
// of NAVI. The iface.Transport.MCP.Server field is treated as the bridge URL.
// The bridge is expected to accept JSON-RPC 2.0 requests on POST and return a
// standard MCP tools/call response. This keeps NAVI decoupled from the MCP
// wire protocol while still exercising the Capability Layer design.
func executeMCPTool(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (string, error) {
	if iface.Transport.MCP == nil || strings.TrimSpace(iface.Transport.MCP.Server) == "" || strings.TrimSpace(iface.Transport.MCP.Tool) == "" {
		return "", fmt.Errorf("mcp_tool transport missing server or tool")
	}

	timeoutMS := entry.Spec.Performance.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultPythonTimeoutMS
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	type mcpCallParams struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	type mcpCallRequest struct {
		JSONRPC string        `json:"jsonrpc"`
		ID      string        `json:"id"`
		Method  string        `json:"method"`
		Params  mcpCallParams `json:"params"`
	}

	payload := mcpCallRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "tools/call",
		Params: mcpCallParams{
			Name:      iface.Transport.MCP.Tool,
			Arguments: args,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "MarshalError", err.Error()))
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, iface.Transport.MCP.Server, bytes.NewReader(bodyBytes))
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "RequestError", err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		status := "error"
		errType := "RequestError"
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			status = "timeout"
			errType = "TimeoutError"
		}
		return marshalExecutionResult(errorExecutionResult(status, entry, iface, errType, err.Error()))
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, int64(defaultMaxOutputBytes)))
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "ReadError", err.Error()))
	}

	var decoded map[string]any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &decoded); err != nil {
			decoded = map[string]any{
				"raw": string(rawBody),
			}
		}
	}

	result := SkillExecutionResult{
		Status:     "success",
		Payload:    decoded,
		DurationMS: time.Since(start).Milliseconds(),
		Metadata: SkillResultMetadata{
			SkillID:   entry.Spec.SkillID,
			Interface: iface.Name,
		},
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Status = "error"
		result.Error = &SkillResultError{
			Type:    "HTTPError",
			Message: fmt.Sprintf("mcp bridge request failed with status %d", resp.StatusCode),
		}
	}
	return marshalExecutionResult(result)
}

func executeInternal(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (string, error) {
	if entry.Spec == nil {
		return "", ErrNotExecutable
	}

	handler, ok := GetInternalHandler(entry.Spec.SkillID, iface.Name)
	if !ok {
		return "", fmt.Errorf("%w: internal handler not registered for %s/%s", ErrNotExecutable, entry.Spec.SkillID, iface.Name)
	}

	start := time.Now()
	payload, err := handler(ctx, entry, iface, args)
	duration := time.Since(start).Milliseconds()

	result := SkillExecutionResult{
		DurationMS: duration,
		Metadata: SkillResultMetadata{
			SkillID:   entry.Spec.SkillID,
			Interface: iface.Name,
		},
	}

	if err != nil {
		result.Status = "error"
		result.Error = &SkillResultError{
			Type:    "InternalHandlerError",
			Message: err.Error(),
		}
	} else {
		result.Status = "success"
		result.Payload = payload
	}

	return marshalExecutionResult(result)
}

func executeREST(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (string, error) {
	if iface.Transport.REST == nil {
		return "", fmt.Errorf("rest transport missing configuration")
	}

	timeoutMS := entry.Spec.Performance.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultPythonTimeoutMS
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	method := strings.ToUpper(strings.TrimSpace(iface.Transport.REST.Method))
	if method == "" {
		method = http.MethodPost
	}

	targetURL := iface.Transport.REST.URL
	var body io.Reader
	if method == http.MethodGet {
		u, err := url.Parse(targetURL)
		if err != nil {
			return marshalExecutionResult(errorExecutionResult("error", entry, iface, "RequestError", err.Error()))
		}
		q := u.Query()
		for k, v := range args {
			q.Set(k, fmt.Sprint(v))
		}
		u.RawQuery = q.Encode()
		targetURL = u.String()
	} else {
		payload, err := json.Marshal(args)
		if err != nil {
			return marshalExecutionResult(errorExecutionResult("error", entry, iface, "MarshalError", err.Error()))
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, targetURL, body)
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "RequestError", err.Error()))
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range iface.Transport.REST.Auth {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		status := "error"
		errType := "RequestError"
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			status = "timeout"
			errType = "TimeoutError"
		}
		return marshalExecutionResult(errorExecutionResult(status, entry, iface, errType, err.Error()))
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, int64(defaultMaxOutputBytes)))
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "ReadError", err.Error()))
	}

	var payload any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			payload = string(rawBody)
		}
	}

	result := SkillExecutionResult{
		Status:     "success",
		Payload:    payload,
		DurationMS: time.Since(start).Milliseconds(),
		Metadata: SkillResultMetadata{
			SkillID:   entry.Spec.SkillID,
			Interface: iface.Name,
		},
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Status = "error"
		result.Error = &SkillResultError{
			Type:    "HTTPError",
			Message: fmt.Sprintf("request failed with status %d", resp.StatusCode),
		}
	}
	return marshalExecutionResult(result)
}

func marshalExecutionResult(result SkillExecutionResult) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func errorExecutionResult(status string, entry *SkillEntry, iface *Interface, errType, msg string) SkillExecutionResult {
	result := SkillExecutionResult{
		Status: status,
		Error: &SkillResultError{
			Type:    errType,
			Message: msg,
		},
	}
	if entry != nil && entry.Spec != nil {
		result.Metadata.SkillID = entry.Spec.SkillID
	}
	if iface != nil {
		result.Metadata.Interface = iface.Name
	}
	return result
}
