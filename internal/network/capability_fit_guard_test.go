package network

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type capabilityReadFixture struct{ clarificationFixture }

func (h capabilityReadFixture) path(owner bool) string {
	return strings.ReplaceAll(h.clarificationFixture.path(owner), "/constraint-assessments", "/capability-fit")
}
func (h capabilityReadFixture) read(t *testing.T, owner bool) map[string]any {
	t.Helper()
	var code int
	var out map[string]any
	if owner {
		code, out = ownerCallMatch(h.owner, "GET", h.path(true), "", "", nil)
	} else {
		code, out = h.owner.call("GET", h.path(false), "", nil)
	}
	if code != 200 {
		t.Fatal("current capability read", code, out)
	}
	return out
}
func TestCapabilityFitRequireCurrentSourceOwnershipAndState(t *testing.T) {
	for _, state := range []string{"foreign", "paused", "withdrawn", "expired", "agent_revoked", "principal_suspended"} {
		t.Run(state, func(t *testing.T) {
			h := capabilityReadFixture{newClarificationFixture(t)}
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
				if out["agents"] != nil || out["artifacts"] != nil {
					t.Fatal("denial retained report data", out)
				}
			}
		})
	}
}

func TestCapabilityFitScopeAndOwnerAttributionRemainDistinct(t *testing.T) {
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
	suffix := source["id"].(string) + "/capability-fit"
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

func TestCapabilityFitSourceLockHasBoundedRollbackAndReadRecovery(t *testing.T) {
	h := capabilityReadFixture{newClarificationFixture(t)}
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
