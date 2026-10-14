package network

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type reportReference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type reportInput struct {
	Kind      string           `json:"kind"`
	Subject   reportReference  `json:"subject"`
	Evidence  *reportReference `json:"evidence_ref,omitempty"`
	Statement string           `json:"statement"`
}

var reportReferenceKinds = []string{"principal", "agent", "intent", "swarm", "membership", "message", "plan", "task", "artifact", "evaluation", "opportunity"}

func validReportReference(ref reportReference) bool {
	return slices.Contains(reportReferenceKinds, ref.Kind) && validID(ref.ID)
}

func (s *Service) reportCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if r.URL.Path == "/v1/reports" {
		if r.Method != http.MethodPost {
			return 0, nil, fail(405, "method_not_allowed")
		}
		if !slices.Contains(a.Scopes, "reports:write") {
			return 0, nil, fail(403, "insufficient_scope")
		}
		return idempotent(r, tx, a, func() (int, any, error) {
			var in reportInput
			if err := decode(r, &in); err != nil {
				return 0, nil, err
			}
			if in.Kind != "abuse" && in.Kind != "bug" && in.Kind != "privacy" && in.Kind != "rights" {
				return 0, nil, fail(422, "invalid_report_kind")
			}
			if !validReportReference(in.Subject) || (in.Evidence != nil && !validReportReference(*in.Evidence)) {
				return 0, nil, fail(422, "invalid_report_reference")
			}
			if len(in.Statement) == 0 || len(in.Statement) > 4000 || strings.TrimSpace(in.Statement) == "" {
				return 0, nil, fail(422, "invalid_report_statement")
			}
			if s.reportTriageWindow < time.Hour || s.reportTriageWindow > 30*24*time.Hour {
				return 0, nil, fail(503, "report_intake_unconfigured")
			}
			var recent int
			if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM reports
 WHERE reporter_principal_id=$1 AND created_at>clock_timestamp()-interval '1 minute'`, a.PrincipalID).Scan(&recent); err != nil {
				return 0, nil, err
			}
			if recent >= 30 {
				return 0, nil, fail(429, "report_write_rate_exceeded")
			}
			id := NewID()
			var evidenceKind, evidenceID any
			if in.Evidence != nil {
				evidenceKind, evidenceID = in.Evidence.Kind, in.Evidence.ID
			}
			var created time.Time
			err := tx.QueryRow(r.Context(), `INSERT INTO reports(id,reporter_principal_id,reporter_agent_id,kind,subject_kind,subject_id,
	 evidence_kind,evidence_id,statement,triage_due_at,created_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,clock_timestamp()+($10::bigint*interval '1 second'),clock_timestamp()) RETURNING created_at`,
				id, a.PrincipalID, a.AgentID, in.Kind, in.Subject.Kind, in.Subject.ID, evidenceKind, evidenceID, in.Statement,
				int64(s.reportTriageWindow/time.Second)).Scan(&created)
			if err != nil {
				return 0, nil, err
			}
			origin := "registered"
			if s.environment != "production" {
				origin = "release_test"
			}
			facts := canonical(map[string]string{"kind": in.Kind, "state": "pending"})
			if _, err = tx.Exec(r.Context(), `INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin)
 VALUES($1,'report',$2,1,$3,$4,'report.submitted',$5,$6)`, NewID(), id, a.PrincipalID, a.AgentID, facts, origin); err != nil {
				return 0, nil, err
			}
			return 201, map[string]any{"id": id, "kind": in.Kind, "state": "pending", "created_at": created}, nil
		})
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 || !validID(parts[3]) {
		return 0, nil, fail(404, "not_found")
	}
	if r.Method != http.MethodGet {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if !slices.Contains(a.Scopes, "reports:read") {
		return 0, nil, fail(403, "insufficient_scope")
	}
	var kind, subjectKind, subjectID, statement, state string
	var evidenceKind, evidenceID *string
	var created time.Time
	err := tx.QueryRow(r.Context(), `SELECT kind,subject_kind,subject_id::text,evidence_kind,evidence_id::text,statement,state,created_at
 FROM reports WHERE id=$1`, parts[3]).Scan(&kind, &subjectKind, &subjectID, &evidenceKind, &evidenceID, &statement, &state, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	var evidence any
	if evidenceKind != nil && evidenceID != nil {
		evidence = reportReference{Kind: *evidenceKind, ID: *evidenceID}
	}
	return 200, map[string]any{
		"id": parts[3], "kind": kind, "subject": reportReference{Kind: subjectKind, ID: subjectID},
		"evidence_ref": evidence, "statement": statement, "state": state, "created_at": created,
	}, nil
}
