package network

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type clarificationFixture struct {
	service           *Service
	db                *pgx.Conn
	owner, publisher  testClient
	source, candidate map[string]any
	publishedInput    IntentInput
}

// Retirement uses ordinary owner commands; it does not delete evidence or repair
// migration ledgers. Non-current fault-injected records remain retained.
func retireClarificationFixture(t *testing.T, db *pgx.Conn, owner testClient, id string) {
	t.Helper()
	var state string
	var version int64
	var current bool
	err := db.QueryRow(context.Background(), `SELECT i.state,i.version,a.status='active' AND p.status='active' AND i.expires_at>clock_timestamp() FROM undergrid.intents i JOIN undergrid.agents a ON a.id=i.agent_id JOIN undergrid.principals p ON p.id=i.principal_id WHERE i.id=$1`, id).Scan(&state, &version, &current)
	if err != nil {
		t.Error(err)
		return
	}
	if !current || state != "active" {
		return
	}
	code, out := ownerCallMatch(owner, "PATCH", "/v1/owner/intents/"+id, NewID(), etag(version), map[string]any{"state": "withdrawn"})
	if code != 200 {
		t.Errorf("retire owned fixture: %d %v", code, out)
	}
}
func newClarificationFixture(t *testing.T) clarificationFixture {
	t.Helper()
	s, db := ownerSwarmCreationFixture(t)
	h := clarificationFixture{service: s, db: db, owner: client(t, s), publisher: client(t, s)}
	input := intentFixture()
	input.Objective = "Current declared coverage " + NewID()
	input.Watch.Enabled = false
	input.HardConstraints.Coverage = &CoverageConstraint{Required: []string{"region:eu"}, Excluded: []string{}}
	code, row := h.owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, row)
	}
	h.source = row
	h.publishedInput = intentFixture()
	h.publishedInput.Objective = input.Objective
	h.publishedInput.PublicSummary = input.Objective
	h.publishedInput.Visibility = "redacted_public"
	code, row = h.publisher.call("POST", "/v1/intents", NewID(), h.publishedInput)
	if code != 201 {
		t.Fatal(code, row)
	}
	h.candidate = row
	t.Cleanup(func() {
		retireClarificationFixture(t, db, h.owner, h.source["id"].(string))
		retireClarificationFixture(t, db, h.publisher, h.candidate["id"].(string))
	})
	return h
}
func (h clarificationFixture) path(owner bool) string {
	prefix := "/v1/intents/"
	if owner {
		prefix = "/v1/owner/intents/"
	}
	return prefix + h.source["id"].(string) + "/constraint-assessments"
}
func (h clarificationFixture) read(t *testing.T, owner bool) map[string]any {
	t.Helper()
	var code int
	var report map[string]any
	if owner {
		code, report = ownerCallMatch(h.owner, "GET", h.path(true), "", "", nil)
	} else {
		code, report = h.owner.call("GET", h.path(false), "", nil)
	}
	if code != 200 {
		t.Fatalf("current report: %d %v", code, report)
	}
	return report
}
func reportedCandidate(t *testing.T, report map[string]any, id string) map[string]any {
	t.Helper()
	rows, ok := report["items"].([]any)
	if !ok {
		t.Fatal("missing finite items", report)
	}
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatal(raw)
		}
		if row["candidate_intent_id"] == id {
			return row
		}
	}
	return nil
}

func TestConstraintAssessmentsRequireCurrentSourceOwnershipAndState(t *testing.T) {
	for _, state := range []string{"foreign", "paused", "withdrawn", "expired", "agent_revoked", "principal_suspended"} {
		t.Run(state, func(t *testing.T) {
			h := newClarificationFixture(t)
			h.read(t, true)
			if state == "foreign" {
				foreign := client(t, h.service)
				for _, owner := range []bool{false, true} {
					code, out := foreign.call("GET", h.path(owner), "", nil)
					if code != 404 {
						t.Fatal("foreign source disclosure", code, out)
					}
				}
				return
			}
			if state == "paused" || state == "withdrawn" {
				code, out := h.owner.versioned("PATCH", "/v1/intents/"+h.source["id"].(string), NewID(), h.source["etag"].(string), map[string]any{"state": state})
				if code != 200 {
					t.Fatal(code, out)
				}
			} else {
				query := `UPDATE undergrid.intents SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`
				if state == "agent_revoked" {
					query = `UPDATE undergrid.agents SET status='revoked' WHERE id=(SELECT agent_id FROM undergrid.intents WHERE id=$1)`
				}
				if state == "principal_suspended" {
					query = `UPDATE undergrid.principals SET status='suspended' WHERE id=(SELECT principal_id FROM undergrid.intents WHERE id=$1)`
				}
				if _, err := h.db.Exec(context.Background(), query, h.source["id"]); err != nil {
					t.Fatal(err)
				}
			}
			for _, owner := range []bool{false, true} {
				code, out := h.owner.call("GET", h.path(owner), "", nil)
				if state == "agent_revoked" || state == "principal_suspended" {
					baseline := "/v1/me"
					if owner {
						baseline = "/v1/owner/intents/" + h.source["id"].(string)
					}
					expected, _ := h.owner.call("GET", baseline, "", nil)
					if code == 200 || code != expected {
						t.Fatal("revoked attribution differed from existing identity gate", code, expected, out)
					}
				} else if code != 404 {
					t.Fatal("noncurrent source disclosed", code, out)
				}
				if out["items"] != nil {
					t.Fatal("denial retained report data", out)
				}
			}
		})
	}
}

func TestConstraintAssessmentsRecheckPublisherEvidenceAndResolveRevision(t *testing.T) {
	t.Run("revision", func(t *testing.T) {
		h := newClarificationFixture(t)
		id := h.candidate["id"].(string)
		if row := reportedCandidate(t, h.read(t, true), id); row == nil || row["hard_status"] != "unknown" {
			t.Fatal(row)
		}
		input := h.publishedInput
		input.HardConstraints.Coverage = &CoverageConstraint{Required: []string{"region:eu"}, Excluded: []string{}}
		code, out := h.publisher.versioned("PATCH", "/v1/intents/"+id, NewID(), h.candidate["etag"].(string), map[string]any{"revision": input})
		if code != 200 {
			t.Fatal(code, out)
		}
		row := reportedCandidate(t, h.read(t, true), id)
		if row == nil || row["candidate_version"] != float64(2) || row["hard_status"] != "aligned" || row["clarification_required"] != false {
			t.Fatal("new declaration did not resolve current gap", row)
		}
	})
	for _, state := range []string{"paused", "withdrawn", "expired", "agent_revoked", "principal_suspended"} {
		t.Run(state, func(t *testing.T) {
			h := newClarificationFixture(t)
			id := h.candidate["id"].(string)
			if reportedCandidate(t, h.read(t, true), id) == nil {
				t.Fatal("missing current public precondition")
			}
			if state == "paused" || state == "withdrawn" {
				code, out := h.publisher.versioned("PATCH", "/v1/intents/"+id, NewID(), h.candidate["etag"].(string), map[string]any{"state": state})
				if code != 200 {
					t.Fatal(code, out)
				}
			} else {
				query := `UPDATE undergrid.intents SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`
				if state == "agent_revoked" {
					query = `UPDATE undergrid.agents SET status='revoked' WHERE id=(SELECT agent_id FROM undergrid.intents WHERE id=$1)`
				}
				if state == "principal_suspended" {
					query = `UPDATE undergrid.principals SET status='suspended' WHERE id=(SELECT principal_id FROM undergrid.intents WHERE id=$1)`
				}
				if _, err := h.db.Exec(context.Background(), query, id); err != nil {
					t.Fatal(err)
				}
			}
			for _, owner := range []bool{false, true} {
				if row := reportedCandidate(t, h.read(t, owner), id); row != nil {
					t.Fatal("stale public evidence disclosed", row)
				}
			}
		})
	}
}

func TestConstraintAssessmentsScopeAndOwnerAttributionRemainDistinct(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner := client(t, s)
	code, issued := owner.call("POST", "/v1/agents", NewID(), map[string]any{"name": "write-only agent", "scopes": []string{"intents:write"}})
	if code != 201 {
		t.Fatal(code, issued)
	}
	writer := owner
	writer.token = issued["token"].(string)
	code, source := writer.call("POST", "/v1/intents", NewID(), intentFixture())
	if code != 201 {
		t.Fatal(code, source)
	}
	t.Cleanup(func() { retireClarificationFixture(t, db, owner, source["id"].(string)) })
	suffix := source["id"].(string) + "/constraint-assessments"
	if code, out := writer.call("GET", "/v1/intents/"+suffix, "", nil); code != 403 {
		t.Fatal("write scope became read authority", code, out)
	}
	if code, out := ownerCallMatch(owner, "GET", "/v1/owner/intents/"+suffix, "", "", nil); code != 200 {
		t.Fatal("owner could not attribute existing active owned source", code, out)
	}
	if code, out := owner.call("GET", "/v1/intents/"+suffix, "", nil); code != 404 {
		t.Fatal("sibling agent borrowed source attribution", code, out)
	}
}

func TestConstraintAssessmentSourceLockHasBoundedRollbackAndReadRecovery(t *testing.T) {
	h := newClarificationFixture(t)
	h.read(t, true)
	held, err := h.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(context.Background())
	if _, err = held.Exec(context.Background(), `SELECT 1 FROM undergrid.intents WHERE id=$1 FOR UPDATE`, h.source["id"]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, h.path(true), nil).WithContext(ctx)
	request.Header.Set("X-Test-Owner", h.owner.owner)
	recorder := httptest.NewRecorder()
	started := time.Now()
	h.service.ServeHTTP(recorder, request)
	var result map[string]any
	if err = json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	failure, ok := result["error"].(map[string]any)
	if recorder.Code != 500 || !ok || failure["retryable"] != true || time.Since(started) > 2*time.Second {
		t.Fatal("authority contention must roll back promptly", recorder.Code, result, time.Since(started))
	}
	if err = held.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.read(t, true)
}

func TestConstraintAssessmentsBoundDistinctForeignPrincipals(t *testing.T) {
	h := newClarificationFixture(t)
	publish := func(owner testClient) {
		t.Helper()
		code, row := owner.call("POST", "/v1/intents", NewID(), h.publishedInput)
		if code != 201 {
			t.Fatal(code, row)
		}
		id := row["id"].(string)
		t.Cleanup(func() { retireClarificationFixture(t, h.db, owner, id) })
	}
	// Multiple public declarations by one publisher must not crowd out others;
	// the source owner's public declarations are not foreign evidence.
	publish(h.publisher)
	publish(h.owner)
	publish(client(t, h.service))
	check := func(want int, truncated bool) {
		t.Helper()
		report := h.read(t, true)
		rows := report["items"].([]any)
		if len(rows) != want || report["candidate_limit"] != float64(20) || report["truncated"] != truncated {
			t.Fatalf("bounded report: %v", report)
		}
		seen := map[string]bool{}
		var sourcePrincipal string
		if err := h.db.QueryRow(context.Background(), `SELECT principal_id FROM undergrid.intents WHERE id=$1`, h.source["id"]).Scan(&sourcePrincipal); err != nil {
			t.Fatal(err)
		}
		for _, raw := range rows {
			row := raw.(map[string]any)
			principal := row["principal_id"].(string)
			if principal == sourcePrincipal || seen[principal] {
				t.Fatalf("nonforeign or duplicate principal: %v", row)
			}
			seen[principal] = true
		}
	}
	check(2, false)
	for range 19 {
		publish(client(t, h.service))
	}
	check(20, true)
}
