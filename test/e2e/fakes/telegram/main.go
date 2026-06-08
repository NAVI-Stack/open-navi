package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type mode struct {
	HealthStatus int `json:"health_status,omitempty"`
	SendStatus   int `json:"send_status,omitempty"`
	EditStatus   int `json:"edit_status,omitempty"`
	DelayMS      int `json:"delay_ms,omitempty"`
	PollWaitMS   int `json:"poll_wait_ms,omitempty"`
}

type inboundMessage struct {
	ChatID          int64  `json:"chat_id"`
	Text            string `json:"text"`
	MessageID       int64  `json:"message_id,omitempty"`
	MessageThreadID int64  `json:"message_thread_id,omitempty"`
}

type outboundRecord struct {
	Timestamp       time.Time `json:"timestamp"`
	Method          string    `json:"method"`
	ChatID          int64     `json:"chat_id"`
	Text            string    `json:"text,omitempty"`
	MessageID       int64     `json:"message_id,omitempty"`
	MessageThreadID int64     `json:"message_thread_id,omitempty"`
}

type requestRecord struct {
	Timestamp time.Time      `json:"timestamp"`
	Action    string         `json:"action"`
	Method    string         `json:"method"`
	Query     map[string]any `json:"query,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

type update struct {
	UpdateID int64 `json:"update_id"`
	Message  struct {
		MessageID       int64  `json:"message_id"`
		MessageThreadID int64  `json:"message_thread_id,omitempty"`
		Text            string `json:"text"`
		Chat            struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

type state struct {
	mu          sync.Mutex
	mode        mode
	nextUpdate  int64
	nextMessage int64
	updates     []update
	outbound    []outboundRecord
	requests    []requestRecord
}

func newState() *state {
	return &state{
		mode: mode{
			PollWaitMS: 200,
		},
		nextUpdate:  1,
		nextMessage: 1000,
		updates:     make([]update, 0, 32),
		outbound:    make([]outboundRecord, 0, 32),
		requests:    make([]requestRecord, 0, 64),
	}
}

func main() {
	st := newState()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/__admin/reset", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		st.mode = mode{PollWaitMS: 200}
		st.nextUpdate = 1
		st.nextMessage = 1000
		st.updates = st.updates[:0]
		st.outbound = st.outbound[:0]
		st.requests = st.requests[:0]
		st.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/__admin/mode", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			st.mu.Lock()
			current := st.mode
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, current)
		case http.MethodPost:
			var next mode
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.mu.Lock()
			if next.PollWaitMS == 0 {
				next.PollWaitMS = 200
			}
			st.mode = next
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/__admin/inbound", func(w http.ResponseWriter, r *http.Request) {
		var msg inboundMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		st.mu.Lock()
		upd := update{UpdateID: st.nextUpdate}
		st.nextUpdate++
		if msg.MessageID != 0 {
			upd.Message.MessageID = msg.MessageID
		} else {
			upd.Message.MessageID = st.nextMessage
			st.nextMessage++
		}
		upd.Message.MessageThreadID = msg.MessageThreadID
		upd.Message.Text = msg.Text
		upd.Message.Chat.ID = msg.ChatID
		st.updates = append(st.updates, upd)
		st.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/__admin/outbound", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		out := append([]outboundRecord(nil), st.outbound...)
		st.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("/__admin/requests", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		out := append([]requestRecord(nil), st.requests...)
		st.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if !strings.HasPrefix(path, "bot") {
			http.NotFound(w, r)
			return
		}
		action := path[strings.LastIndex(path, "/")+1:]

		st.mu.Lock()
		currentMode := st.mode
		st.mu.Unlock()
		if currentMode.DelayMS > 0 {
			time.Sleep(time.Duration(currentMode.DelayMS) * time.Millisecond)
		}

		switch action {
		case "getMe":
			st.recordRequest(action, r.Method, nil, nil)
			if currentMode.HealthStatus >= 400 {
				writeJSON(w, currentMode.HealthStatus, map[string]any{"ok": false, "description": "health degraded"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true,
				"result": map[string]any{
					"id":         1,
					"is_bot":     true,
					"first_name": "Fake Navi Bot",
					"username":   "fake_navi_bot",
				},
			})
		case "getUpdates":
			offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
			st.recordRequest(action, r.Method, map[string]any{"offset": offset}, nil)
			if currentMode.PollWaitMS > 0 {
				time.Sleep(time.Duration(currentMode.PollWaitMS) * time.Millisecond)
			}
			st.mu.Lock()
			result := make([]update, 0, len(st.updates))
			for _, upd := range st.updates {
				if upd.UpdateID >= offset {
					result = append(result, upd)
				}
			}
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
		case "sendMessage":
			var req struct {
				ChatID          int64  `json:"chat_id"`
				Text            string `json:"text"`
				MessageThreadID int64  `json:"message_thread_id,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.recordRequest(action, r.Method, nil, map[string]any{
				"chat_id":           req.ChatID,
				"text":              req.Text,
				"message_thread_id": req.MessageThreadID,
			})
			if currentMode.SendStatus >= 400 {
				writeJSON(w, currentMode.SendStatus, map[string]any{"ok": false, "description": "send failure"})
				return
			}
			st.mu.Lock()
			messageID := st.nextMessage
			st.nextMessage++
			st.outbound = append(st.outbound, outboundRecord{
				Timestamp:       time.Now().UTC(),
				Method:          "sendMessage",
				ChatID:          req.ChatID,
				Text:            req.Text,
				MessageID:       messageID,
				MessageThreadID: req.MessageThreadID,
			})
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": messageID,
				},
			})
		case "editMessageText":
			var req struct {
				ChatID    int64  `json:"chat_id"`
				MessageID int64  `json:"message_id"`
				Text      string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.recordRequest(action, r.Method, nil, map[string]any{
				"chat_id":    req.ChatID,
				"message_id": req.MessageID,
				"text":       req.Text,
			})
			if currentMode.EditStatus >= 400 {
				writeJSON(w, currentMode.EditStatus, map[string]any{"ok": false, "description": "edit failure"})
				return
			}
			st.mu.Lock()
			st.outbound = append(st.outbound, outboundRecord{
				Timestamp: time.Now().UTC(),
				Method:    "editMessageText",
				ChatID:    req.ChatID,
				Text:      req.Text,
				MessageID: req.MessageID,
			})
			st.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": true})
		case "setWebhook", "deleteWebhook":
			st.recordRequest(action, r.Method, nil, nil)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": true})
		default:
			http.NotFound(w, r)
		}
	})

	log.Println("fake telegram listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (st *state) recordRequest(action, method string, query, body map[string]any) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.requests = append(st.requests, requestRecord{
		Timestamp: time.Now().UTC(),
		Action:    action,
		Method:    method,
		Query:     query,
		Body:      body,
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
