package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type chatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

type requestRecord struct {
	Timestamp time.Time   `json:"timestamp"`
	Request   chatRequest `json:"request"`
}

type scenario struct {
	Mode       string   `json:"mode"`
	Models     []string `json:"models,omitempty"`
	Chunks     []string `json:"chunks,omitempty"`
	DelayMS    int      `json:"delay_ms,omitempty"`
	StatusCode int      `json:"status_code,omitempty"`
	Message    string   `json:"message,omitempty"`
	ToolName   string   `json:"tool_name,omitempty"`
}

type state struct {
	mu       sync.Mutex
	scenario scenario
	requests []requestRecord
}

func newState() *state {
	return &state{
		scenario: scenario{
			Mode:   "happy",
			Models: []string{"llama3:latest", "mistral:latest"},
		},
		requests: make([]requestRecord, 0, 32),
	}
}

func main() {
	st := newState()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		models := append([]string(nil), st.scenario.Models...)
		st.mu.Unlock()
		type model struct {
			Name string `json:"name"`
		}
		out := struct {
			Models []model `json:"models"`
		}{Models: make([]model, 0, len(models))}
		for _, name := range models {
			out.Models = append(out.Models, model{Name: name})
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		st.mu.Lock()
		st.requests = append(st.requests, requestRecord{Timestamp: time.Now().UTC(), Request: req})
		sc := st.scenario
		st.mu.Unlock()

		if !contains(sc.Models, req.Model) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("model %q not found", req.Model))
			return
		}

		switch sc.Mode {
		case "model-not-found":
			writeError(w, http.StatusNotFound, fmt.Sprintf("model %q not found", req.Model))
			return
		case "timeout":
			time.Sleep(45 * time.Second)
			return
		case "error", "error-401", "error-403", "error-429", "error-500":
			status := sc.StatusCode
			if status == 0 {
				switch sc.Mode {
				case "error-401":
					status = http.StatusUnauthorized
				case "error-403":
					status = http.StatusForbidden
				case "error-429":
					status = http.StatusTooManyRequests
				default:
					status = http.StatusInternalServerError
				}
			}
			writeError(w, status, nonEmpty(sc.Message, "fake ollama error"))
			return
		}

		if req.Stream {
			writeStream(w, sc, req)
			return
		}
		if sc.Mode == "tool-call" {
			toolName := nonEmpty(sc.ToolName, "navi.files.list")
			writeJSON(w, http.StatusOK, map[string]any{
				"id": "chatcmpl-fake",
				"choices": []map[string]any{
					{
						"index":         0,
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]any{
								{
									"id":   "call_fake_list",
									"type": "function",
									"function": map[string]any{
										"name":      toolName,
										"arguments": `{"path":"."}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		content := buildContent(sc, req)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chatcmpl-fake",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": content,
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": len(strings.Fields(content)),
			},
		})
	})
	mux.HandleFunc("/__admin/scenario", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			st.mu.Lock()
			sc := st.scenario
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, sc)
		case http.MethodPost:
			var sc scenario
			if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(sc.Models) == 0 {
				sc.Models = []string{"llama3:latest", "mistral:latest"}
			}
			if sc.Mode == "" {
				sc.Mode = "happy"
			}
			st.mu.Lock()
			st.scenario = sc
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/__admin/requests", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		out := append([]requestRecord(nil), st.requests...)
		st.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("/__admin/reset", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		st.scenario = scenario{
			Mode:   "happy",
			Models: []string{"llama3:latest", "mistral:latest"},
		}
		st.requests = st.requests[:0]
		st.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	log.Println("fake ollama listening on :11434")
	log.Fatal(http.ListenAndServe(":11434", mux))
}

func writeStream(w http.ResponseWriter, sc scenario, req chatRequest) {
	if sc.Mode == "tool-call" {
		toolName := nonEmpty(sc.ToolName, "navi.files.list")
		writeSSE(w, map[string]any{
			"choices": []map[string]any{
				{
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_fake_list",
								"type":  "function",
								"function": map[string]any{
									"name":      toolName,
									"arguments": `{"path":"."}`,
								},
							},
						},
					},
					"finish_reason": nil,
				},
			},
		})
		writeSSE(w, map[string]any{
			"choices": []map[string]any{
				{
					"delta":         map[string]any{},
					"finish_reason": "tool_calls",
				},
			},
		})
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	chunks := sc.Chunks
	if len(chunks) == 0 {
		content := buildContent(sc, req)
		chunks = splitChunks(content)
	}
	delay := time.Duration(sc.DelayMS) * time.Millisecond
	if delay <= 0 {
		delay = 150 * time.Millisecond
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	for _, chunk := range chunks {
		writeSSE(w, map[string]any{
			"choices": []map[string]any{
				{
					"delta": map[string]any{
						"content": chunk,
					},
				},
			},
		})
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(delay)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func writeSSE(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func buildContent(sc scenario, req chatRequest) string {
	if sc.Message != "" {
		return sc.Message
	}
	last := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			last = req.Messages[i].Content
			break
		}
	}
	if last == "" {
		last = "hello"
	}
	return fmt.Sprintf("fake-ollama(%s): %s", req.Model, last)
}

func splitChunks(content string) []string {
	words := strings.Fields(content)
	if len(words) < 2 {
		return []string{content}
	}
	mid := len(words) / 2
	return []string{strings.Join(words[:mid], " ") + " ", strings.Join(words[mid:], " ")}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSONWithCode(w, code, map[string]any{
		"error": map[string]any{
			"message": msg,
		},
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	writeJSONWithCode(w, code, payload)
}

func writeJSONWithCode(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
