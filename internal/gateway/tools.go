package gateway

import (
	"net/http"
	"strings"

	navitool "github.com/ceoai/navi/internal/tool"
)

func (s *Server) toolRegistry() *navitool.Registry {
	if s.cfg.ToolRegistry != nil {
		return s.cfg.ToolRegistry
	}
	if s.cfg.Navi != nil {
		return s.cfg.Navi.ToolRegistry()
	}
	return nil
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	reg := s.toolRegistry()
	if reg == nil {
		replyJSON(w, http.StatusOK, map[string]any{"items": []navitool.Tool{}})
		return
	}
	surface := strings.TrimSpace(r.URL.Query().Get("surface"))
	includeHidden := true
	if raw := strings.TrimSpace(r.URL.Query().Get("include_hidden")); raw != "" {
		includeHidden = raw != "false" && raw != "0"
	}
	items := make([]*navitool.Tool, 0)
	for _, tool := range reg.List() {
		if tool == nil {
			continue
		}
		if !includeHidden && tool.Hidden {
			continue
		}
		if !navitool.VisibleOnSurface(tool, surface) {
			continue
		}
		items = append(items, tool)
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleGetTool(w http.ResponseWriter, r *http.Request) {
	reg := s.toolRegistry()
	if reg == nil {
		replyError(w, http.StatusNotFound, "tool registry not configured")
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		replyError(w, http.StatusBadRequest, "tool name required")
		return
	}
	tool, ok := reg.Lookup(name)
	if !ok {
		replyError(w, http.StatusNotFound, "tool not found")
		return
	}
	replyJSON(w, http.StatusOK, tool)
}
