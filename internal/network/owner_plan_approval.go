package network

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Owner sessions select an owned attribution agent, never a credential. Shared
// plan transactions continue to own membership, rights policy and exact votes.
func (s *Service) ownerPlanApproval(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 6 || parts[0] != "v1" || parts[1] != "owner" || parts[2] != "agents" || !validID(parts[3]) || !validID(parts[5]) {
		return 0, nil, fail(404, "not_found")
	}
	list := len(parts) == 7 && parts[4] == "swarms" && parts[6] == "plans"
	review := len(parts) == 6 && parts[4] == "plans"
	vote := len(parts) == 7 && parts[4] == "plans" && parts[6] == "approvals"
	if !list && !review && !vote {
		return 0, nil, fail(404, "not_found")
	}
	if vote && r.Method != "POST" || !vote && r.Method != "GET" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, parts[3])
	if err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"swarms:read", "plans:approve", "rights:accept"}
	if list {
		if err = roomAuthority(r.Context(), tx, a, parts[5], false); err != nil {
			return 0, nil, err
		}
		after := int64(0)
		if raw := r.URL.Query().Get("before_version"); raw != "" {
			after, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || after < 1 {
				return 0, nil, fail(400, "invalid_cursor")
			}
		}
		rows, err := tx.Query(r.Context(), "SELECT id,state,version,content_hash FROM plan_versions WHERE swarm_id=$1 AND ($2::bigint=0 OR version<$2) ORDER BY version DESC LIMIT 101", parts[5], after)
		if err != nil {
			return 0, nil, err
		}
		defer rows.Close()
		items := []any{}
		next := ""
		var last int64
		for rows.Next() {
			var id, state, hash string
			var version int64
			if err = rows.Scan(&id, &state, &version, &hash); err != nil {
				return 0, nil, err
			}
			if len(items) == 100 {
				next = strconv.FormatInt(last, 10)
				break
			}
			items = append(items, map[string]any{"id": id, "state": state, "version": version, "content_hash": hash})
			last = version
		}
		return 200, map[string]any{"items": items, "next_before_version": next}, rows.Err()
	}
	p, err := loadPlan(r.Context(), tx, parts[5], false)
	if err != nil {
		return 0, nil, err
	}
	if err = roomAuthority(r.Context(), tx, a, p.swarm, false); err != nil {
		return 0, nil, err
	}
	if review {
		result, err := planResult(r.Context(), tx, p)
		if err != nil {
			return 0, nil, err
		}
		out := result.(map[string]any)
		out["contribution_template_hash"] = p.sealed.TemplateHash
		out["terms_url"] = ""
		if template, ok := s.templates[p.sealed.Plan.Rights.TemplateID]; ok && template.SHA256 == p.sealed.TemplateHash {
			out["terms_url"] = template.TermsURL
		}
		return 200, out, nil
	}
	// Authorization above runs before retained command recovery, so removed
	// members, inactive agents and revoked owner sessions cannot replay a vote.
	return idempotent(r, tx, a, func() (int, any, error) {
		var in struct {
			Decision    string `json:"decision"`
			Hash        string `json:"content_hash"`
			Version     int64  `json:"version"`
			Acknowledge bool   `json:"acknowledge_contribution_terms"`
		}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		current, err := loadPlan(r.Context(), tx, p.id, true)
		if err != nil {
			return 0, nil, err
		}
		if in.Version != current.version || in.Hash != current.hash {
			return 0, nil, fail(409, "plan_not_current")
		}
		if in.Decision == "approve" && !in.Acknowledge {
			return 0, nil, fail(422, "contribution_terms_acknowledgement_required")
		}
		command := r.Clone(r.Context())
		command.Body = io.NopCloser(bytes.NewReader(canonical(map[string]string{"decision": in.Decision, "content_hash": in.Hash})))
		return s.approvePlan(command, tx, a, current.id)
	})
}
