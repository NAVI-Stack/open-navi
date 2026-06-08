package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	intakepolicy "github.com/open-navi/navi/internal/intake/policy"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// CIP P5 — intake sync policy + sync log gateway surface.
//
// These endpoints power the Console connector-detail sync state, the policy
// editor, and the (Vault-populated) per-file sync log. Owner policy edits are
// configuration writes, not governed mutations (frozen contract): the Governor
// still decides budgets at runtime; this surface only stores the inputs.

// intakeResolver returns the configured policy resolver, or builds one from DB
// (defaults + runtime overrides) when none was wired.
func (s *Server) intakeResolver() *intakepolicy.Resolver {
	if s.cfg.IntakePolicies != nil {
		return s.cfg.IntakePolicies
	}
	return intakepolicy.NewResolver(s.cfg.DB, nil)
}

// connectorSyncView is the connector-detail sync payload: effective policy, the
// last pass metrics, the next scheduled pass (nil for webhook/manual cadence),
// and recent errors.
type connectorSyncView struct {
	ConnectorID  string                      `json:"connector_id"`
	Policy       schema.SyncPolicy           `json:"policy"`
	LastPass     *schema.IntakeSyncLogEntry  `json:"last_pass,omitempty"`
	NextPass     *time.Time                  `json:"next_pass,omitempty"`
	RecentPasses []schema.IntakeSyncLogEntry `json:"recent_passes"`
	ConsentState string                      `json:"consent_state,omitempty"`
}

func (s *Server) buildConnectorSyncView(r *http.Request, connectorID string) (connectorSyncView, error) {
	ctx := r.Context()
	res := s.intakeResolver()
	pol := res.Resolve(ctx, connectorID)

	view := connectorSyncView{ConnectorID: connectorID, Policy: pol, RecentPasses: []schema.IntakeSyncLogEntry{}}

	recent, err := store.ListIntakeSyncLog(ctx, s.cfg.DB, connectorID, 10)
	if err != nil {
		return view, err
	}
	if recent != nil {
		view.RecentPasses = recent
	}
	if len(recent) > 0 {
		last := recent[0]
		view.LastPass = &last
		// Next scheduled pass = last end + delta cadence, when cadence is an interval.
		if d, ok := pol.Delta.CadenceDuration(); ok && last.EndedAt != nil {
			next := last.EndedAt.Add(d)
			view.NextPass = &next
		}
	}
	if status, _, err := intakepolicy.ConsentState(ctx, s.cfg.DB, connectorID); err == nil && status != "" {
		view.ConsentState = string(status)
	}
	return view, nil
}

// handleGetIntakePolicy returns one connector's sync view (?connector=…) or, with
// no connector param, the sync view for every registered connector.
func (s *Server) handleGetIntakePolicy(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if connector := r.URL.Query().Get("connector"); connector != "" {
		view, err := s.buildConnectorSyncView(r, connector)
		if err != nil {
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, view)
		return
	}

	// List mode: one view per registered connector.
	views := []connectorSyncView{}
	seen := map[string]bool{}
	if s.cfg.Registry != nil {
		for _, c := range s.cfg.Registry.List() {
			if c.Name == "" || seen[c.Name] {
				continue
			}
			seen[c.Name] = true
			view, err := s.buildConnectorSyncView(r, c.Name)
			if err != nil {
				replyError(w, http.StatusInternalServerError, err.Error())
				return
			}
			views = append(views, view)
		}
	}
	// Also surface any connector that has produced sync-log rows but isn't in the
	// live registry (e.g. an account id like "telegram:acct-1").
	if logs, err := store.ListIntakeSyncLog(r.Context(), s.cfg.DB, "", 200); err == nil {
		for _, e := range logs {
			if e.ConnectorID == "" || seen[e.ConnectorID] {
				continue
			}
			seen[e.ConnectorID] = true
			view, err := s.buildConnectorSyncView(r, e.ConnectorID)
			if err != nil {
				continue
			}
			views = append(views, view)
		}
	}
	replyJSON(w, http.StatusOK, views)
}

// handleSetIntakePolicy persists an owner sync-policy override. The body is a
// full schema.SyncPolicy; its non-zero fields overlay the default/file policy at
// resolve time. This is a configuration write, not a governed mutation.
func (s *Server) handleSetIntakePolicy(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	var p schema.SyncPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if p.ConnectorID == "" {
		p.ConnectorID = r.URL.Query().Get("connector")
	}
	res := s.intakeResolver()
	if err := res.SaveOverride(r.Context(), p); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Return the new effective policy so the editor round-trips visibly.
	view, err := s.buildConnectorSyncView(r, p.ConnectorID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, view)
}

// handleIntakeSyncLog lists intake sync-log rows (?connector=…&limit=…).
func (s *Server) handleIntakeSyncLog(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	connector := r.URL.Query().Get("connector")
	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	list, err := store.ListIntakeSyncLog(r.Context(), s.cfg.DB, connector, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []schema.IntakeSyncLogEntry{}
	}
	replyJSON(w, http.StatusOK, list)
}

// handleVaultSyncLog lists Vault sync-log rows (?path=…&limit=…). The Vault
// deliverable populates these; until then the list is empty (the surface exists).
func (s *Server) handleVaultSyncLog(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	path := r.URL.Query().Get("path")
	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	list, err := store.ListVaultSyncLog(r.Context(), s.cfg.DB, path, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []schema.VaultSyncLogEntry{}
	}
	replyJSON(w, http.StatusOK, list)
}

// handleRecentIntake lists recent provenance-bearing intake records
// (?connector=…&limit=…) for the right-inspector recent-intake panel (CIP §11).
func (s *Server) handleRecentIntake(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	connector := r.URL.Query().Get("connector")
	limit := parseLimit(r.URL.Query().Get("limit"), 20)
	list, err := store.ListRecentIntakeRecords(r.Context(), s.cfg.DB, connector, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.IntakeRecordSummary{}
	}
	replyJSON(w, http.StatusOK, list)
}

// handleRequestBackfill raises a backfill consent Proposal for a connector
// (CIP §7.1). The pass does not start until the owner approves the Proposal in
// the existing Proposal queue.
func (s *Server) handleRequestBackfill(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	var req struct {
		Connector string `json:"connector"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Connector == "" {
		replyError(w, http.StatusBadRequest, "connector is required")
		return
	}
	pol := s.intakeResolver().Resolve(r.Context(), req.Connector)
	proposalID, resume, err := intakepolicy.RequestBackfill(r.Context(), s.cfg.DB, req.Connector, pol)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"proposal_id": proposalID,
		"resume":      resume,
		"status":      "consent_pending",
	})
}

func parseLimit(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	if n > 500 {
		n = 500
	}
	return n
}
