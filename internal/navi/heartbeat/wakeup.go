package heartbeat

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

const defaultWakeCoalesce = 250 * time.Millisecond

// WakePriority controls coalescing when multiple wake-ups target the same session key (highest wins).
type WakePriority int

const (
	WakePriorityAction WakePriority = iota
	WakePriorityDefault
	WakePriorityInterval
	WakePriorityRetry
)

type pendingWake struct {
	notes string
	pri   WakePriority
}

// RequestWakeNow requests an immediate heartbeat with default priority (compatible with callers that omit priority).
func (h *HeartbeatService) RequestWakeNow(reason string) {
	h.RequestWake(reason, int(WakePriorityDefault), "")
}

// RequestWake enqueues heartbeat execution after a coalesce window, merging duplicates per sessionKey.
func (h *HeartbeatService) RequestWake(reason string, priority int, sessionKey string) {
	reason = strings.TrimSpace(reason)
	priv := WakePriority(priority)
	if sessionKey == "" {
		sessionKey = schema.HeartbeatAutoRuntimeSessionID
	}

	h.mu.RLock()
	if !h.running || h.handler == nil {
		h.mu.RUnlock()
		return
	}
	h.mu.RUnlock()

	if h.wakeSuppressed() {
		return
	}

	h.wakeMu.Lock()
	defer h.wakeMu.Unlock()

	if h.pendingWake == nil {
		h.pendingWake = make(map[string]*pendingWake)
	}
	prev := h.pendingWake[sessionKey]
	if prev != nil && priv < prev.pri {
		return
	}
	if prev != nil && priv == prev.pri {
		prev.notes = mergeWakeLines(prev.notes, reason)
	} else {
		h.pendingWake[sessionKey] = &pendingWake{notes: reason, pri: priv}
	}

	d := h.wakeCoalesce
	if d <= 0 {
		d = defaultWakeCoalesce
	}
	if h.wakeTimer != nil {
		h.wakeTimer.Stop()
	}
	h.wakeTimer = time.AfterFunc(d, func() {
		h.drainMergedWake()
	})
}

func mergeWakeLines(prev, next string) string {
	if prev == "" {
		return next
	}
	if next == "" || strings.Contains(prev, next) {
		return prev
	}
	return prev + "\n" + next
}

func (h *HeartbeatService) wakeSuppressed() bool {
	h.mu.RLock()
	dnd := h.dndEnabled
	start := h.quietStart
	end := h.quietEnd
	tz := h.timezone
	h.mu.RUnlock()

	if dnd {
		return true
	}

	now := h.now()
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			now = now.In(loc)
		}
	} else {
		now = now.Local()
	}
	return inQuietHours(now, start, end)
}

func (h *HeartbeatService) drainMergedWake() {
	h.wakeMu.Lock()
	runMap := h.pendingWake
	h.pendingWake = nil
	if h.wakeTimer != nil {
		h.wakeTimer.Stop()
		h.wakeTimer = nil
	}
	h.wakeMu.Unlock()

	if len(runMap) == 0 {
		return
	}
	keys := make([]string, 0, len(runMap))
	for k := range runMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h.mu.RLock()
	run := h.running && h.handler != nil
	h.mu.RUnlock()
	if !run {
		return
	}

	for _, k := range keys {
		p := runMap[k]
		if p == nil {
			continue
		}
		notes := strings.TrimSpace(p.notes)
		h.executeHeartbeat(context.Background(), notes)
	}
}
