package network

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func sharingIntent() IntentInput {
	in := revisionInput("Restricted catalog request")
	in.Visibility = "restricted"
	budget := json.RawMessage(`{"amount_minor":12500,"currency":"EUR","kind":"stated_budget"}`)
	in.Budget = &budget
	return in
}

func TestRestrictedIntentGrantSharesOnlyNamedCurrentFields(t *testing.T) {
	s, db := fixture(t)
	owner, recipient, legacyRecipient, stranger := client(t, s), client(t, s), client(t, s), client(t, s)
	input := sharingIntent()
	code, created := owner.call("POST", "/v1/intents", "create-restricted-for-sharing", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	code, _ = recipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 404 {
		t.Fatalf("restricted intent visible without grant: %d", code)
	}

	_, recipientIdentity := recipient.call("GET", "/v1/me", "", nil)
	_, legacyIdentity := legacyRecipient.call("GET", "/v1/me", "", nil)
	grantPath := "/v1/owner/intents/" + id + "/grants"
	grant := map[string]any{
		"principal_id": recipientIdentity["principal_id"],
		"fields":       []string{"objective", "output"},
		"expires_at":   time.Now().Add(12 * time.Hour).UTC(),
	}
	code, grants := ownerCallMatch(owner, "POST", grantPath, "grant-restricted-fields", created["etag"].(string), grant)
	if code != 201 {
		t.Fatalf("grant %d %#v", code, grants)
	}
	code, replay := ownerCallMatch(owner, "POST", grantPath, "grant-restricted-fields", created["etag"].(string), grant)
	if code != 201 || replay["etag"] != grants["etag"] {
		t.Fatalf("grant replay changed result: %d %#v", code, replay)
	}
	restartedOwner := owner
	restartedOwner.s = New(s.dsn, "development", s.ownerAuth)
	code, restartedReplay := ownerCallMatch(restartedOwner, "POST", grantPath, "grant-restricted-fields", created["etag"].(string), grant)
	if code != 201 || restartedReplay["etag"] != grants["etag"] {
		t.Fatalf("grant retry after handler restart changed result: %d %#v", code, restartedReplay)
	}
	code, ownerGrantList := owner.call("GET", grantPath, "", nil)
	if code != 200 || ownerGrantList["etag"] != grants["etag"] || len(ownerGrantList["items"].([]any)) != 1 {
		t.Fatalf("owner grant list %d %#v", code, ownerGrantList)
	}
	listed := ownerGrantList["items"].([]any)[0].(map[string]any)
	if listed["status"] != "current" || listed["principal_id"] != recipientIdentity["principal_id"] {
		t.Fatalf("owner grant list omitted current record: %#v", listed)
	}
	changedBody := map[string]any{"principal_id": recipientIdentity["principal_id"], "fields": []string{"objective"}, "expires_at": grant["expires_at"]}
	code, _ = ownerCallMatch(owner, "POST", grantPath, "grant-restricted-fields", created["etag"].(string), changedBody)
	if code != 409 {
		t.Fatalf("changed replay body accepted: %d", code)
	}
	code, _ = ownerCallMatch(owner, "POST", grantPath, "missing-grant-etag", "", grant)
	if code != 428 {
		t.Fatalf("grant without ETag accepted: %d", code)
	}
	code, _ = ownerCallMatch(owner, "POST", grantPath, "stale-grant-etag", `"v999"`, grant)
	if code != 409 {
		t.Fatalf("stale grant ETag accepted: %d", code)
	}
	code, _ = ownerCallMatch(stranger, "POST", grantPath, "foreign-intent-grant", created["etag"].(string), grant)
	if code != 404 {
		t.Fatalf("foreign owner learned or changed grant: %d", code)
	}

	code, shared := recipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 200 {
		t.Fatalf("recipient could not read granted fields: %d %#v", code, shared)
	}
	if shared["objective"] != input.Objective || shared["output"] == nil || shared["grant_expires_at"] == nil {
		t.Fatalf("selected fields missing: %#v", shared)
	}
	for _, forbidden := range []string{"budget", "hard_constraints", "preferences", "watch", "public_summary"} {
		if _, exists := shared[forbidden]; exists {
			t.Fatalf("unselected field %q leaked: %#v", forbidden, shared)
		}
	}
	allowed := map[string]bool{"id": true, "version": true, "state": true, "expires_at": true, "grant_expires_at": true, "etag": true, "objective": true, "output": true}
	for field := range shared {
		if !allowed[field] {
			t.Fatalf("unapproved response key %q leaked: %#v", field, shared)
		}
	}
	var intentVersion int64
	var grantsCount int
	if err := db.QueryRow(context.Background(), "SELECT version FROM undergrid.intents WHERE id=$1", id).Scan(&intentVersion); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_grants WHERE intent_id=$1", id).Scan(&grantsCount); err != nil {
		t.Fatal(err)
	}
	if intentVersion != 2 || grantsCount != 1 {
		t.Fatalf("grant persistence version=%d grants=%d", intentVersion, grantsCount)
	}
	var eventCount, inboxCount, scanJobCount int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1", id).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.aggregate_id=$1", id).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND args->>'cause_id'=$1", id).Scan(&scanJobCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 || inboxCount != 2 || scanJobCount != 1 {
		t.Fatalf("grant retry effects events=%d inbox=%d scan_jobs=%d", eventCount, inboxCount, scanJobCount)
	}
	readerTx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer readerTx.Rollback(context.Background())
	if _, err = readerTx.Exec(context.Background(), "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = readerTx.Exec(context.Background(), "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", recipientIdentity["principal_id"], recipientIdentity["agent_id"]); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"intent_revisions", "intent_grants"} {
		var visible int
		query := "SELECT count(*) FROM " + table + " WHERE intent_id=$1"
		if err = readerTx.QueryRow(context.Background(), query, id).Scan(&visible); err != nil || visible != 0 {
			t.Fatalf("recipient raw %s RLS visible=%d err=%v", table, visible, err)
		}
	}
	if err = readerTx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Budget is a distinct selected field. Updating the grant is explicit and
	// binds it to the same immutable revision under a fresh owner ETag.
	grant["fields"] = []string{"budget", "objective"}
	code, budgetGrant := ownerCallMatch(owner, "POST", grantPath, "explicit-budget-grant", grants["etag"].(string), grant)
	if code != 200 {
		t.Fatalf("explicit budget grant %d %#v", code, budgetGrant)
	}
	code, shared = recipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 200 || shared["budget"] == nil || shared["output"] != nil || shared["hard_constraints"] != nil {
		t.Fatalf("budget selection not exact: %d %#v", code, shared)
	}

	// Revision advances the pointer without deleting history. Its changed
	// revision number makes the old grant inert until owner regrant.
	updated := input
	updated.Objective = "Revised restricted catalog request"
	code, revised := revisionCall(owner, "PATCH", "/v1/intents/"+id, "revise-shared-intent", budgetGrant["etag"].(string), map[string]any{"revision": updated})
	if code != 200 {
		t.Fatalf("revise shared intent %d %#v", code, revised)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 404 {
		t.Fatalf("old revision grant survived edit: %d", code)
	}
	code, _ = legacyRecipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 404 {
		t.Fatalf("unbound legacy grant disclosed current revision: %d", code)
	}
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.intent_grants(intent_id,principal_id,fields,expires_at,revision) VALUES($1,$2,$3,now()+interval '1 hour',NULL)`, id, legacyIdentity["principal_id"], []string{"objective"}); err != nil {
		t.Fatal(err)
	}
	code, _ = legacyRecipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 404 {
		t.Fatalf("legacy NULL-revision row became consent: %d", code)
	}
	legacyGrant := map[string]any{"principal_id": legacyIdentity["principal_id"], "fields": []string{"objective"}, "expires_at": time.Now().Add(6 * time.Hour).UTC()}
	code, rebound := ownerCallMatch(owner, "POST", grantPath, "explicit-legacy-regrant", revised["etag"].(string), legacyGrant)
	if code != 200 || rebound["revision"] != float64(2) {
		t.Fatalf("legacy explicit regrant %d %#v", code, rebound)
	}
	code, legacyShared := legacyRecipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 200 || legacyShared["objective"] != updated.Objective || legacyShared["budget"] != nil {
		t.Fatalf("explicit legacy grant %d %#v", code, legacyShared)
	}

	// Revocation commits in the owner command; a fresh recipient transaction
	// evaluates the current row and gets the same concealed 404 as no grant.
	revokePath := grantPath + "/" + recipientIdentity["principal_id"].(string) + "/revoke"
	code, revoked := ownerCallMatch(owner, "POST", revokePath, "revoke-shared-intent", rebound["etag"].(string), map[string]any{})
	if code != 200 {
		t.Fatalf("revoke grant %d %#v", code, revoked)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+id, "", nil)
	if code != 404 {
		t.Fatalf("revoked grant still readable: %d", code)
	}
	var remaining int
	var expired bool
	if err := db.QueryRow(context.Background(), "SELECT count(*),bool_and(expires_at<=now()) FROM undergrid.intent_grants WHERE intent_id=$1 AND principal_id=$2", id, recipientIdentity["principal_id"]).Scan(&remaining, &expired); err != nil || remaining != 1 || !expired {
		t.Fatalf("revocation did not preserve an expired audit row: rows=%d expired=%v err=%v", remaining, expired, err)
	}
}

func TestAgentCredentialCannotManageIntentGrants(t *testing.T) {
	s, _ := fixture(t)
	owner := client(t, s)
	input := sharingIntent()
	code, created := owner.call("POST", "/v1/intents", "create-restricted-agent-denial", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	r := httptest.NewRequest("POST", "/v1/owner/intents/"+created["id"].(string)+"/grants", nil)
	r.Header.Set("Authorization", "Bearer "+owner.token)
	r.Header.Set("Idempotency-Key", "agent-attempt")
	r.Header.Set("If-Match", created["etag"].(string))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code == 201 || w.Code == 200 {
		t.Fatalf("agent credential wrote an owner grant: %d %s", w.Code, w.Body.String())
	}
}

func TestGrantRequiresRestrictedActiveSourceAllowlistedFieldsAndBoundedExpiry(t *testing.T) {
	s, db := fixture(t)
	owner, recipient := client(t, s), client(t, s)
	input := sharingIntent()
	input.ExpiresAt = time.Now().Add(2 * time.Hour).UTC()
	code, created := owner.call("POST", "/v1/intents", "create-sharing-validation", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	_, target := recipient.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/intents/" + created["id"].(string) + "/grants"
	valid := func(exp time.Time, fields []string) map[string]any {
		return map[string]any{"principal_id": target["principal_id"], "fields": fields, "expires_at": exp.UTC()}
	}
	for name, body := range map[string]map[string]any{
		"empty fields":        valid(time.Now().Add(time.Hour), []string{}),
		"unknown field":       valid(time.Now().Add(time.Hour), []string{"watch"}),
		"duplicate field":     valid(time.Now().Add(time.Hour), []string{"objective", "objective"}),
		"past expiry":         valid(time.Now().Add(-time.Minute), []string{"objective"}),
		"after intent expiry": valid(input.ExpiresAt.Add(time.Minute), []string{"objective"}),
	} {
		code, _ = ownerCallMatch(owner, "POST", path, "invalid-"+strings.ReplaceAll(name, " ", "-"), created["etag"].(string), body)
		if code != 422 {
			t.Fatalf("%s grant accepted: %d", name, code)
		}
	}
	var grantCount int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_grants WHERE intent_id=$1", created["id"]).Scan(&grantCount); err != nil || grantCount != 0 {
		t.Fatalf("invalid grants persisted: %d %v", grantCount, err)
	}

	input.Visibility = "private"
	code, private := owner.call("POST", "/v1/intents", "create-private-not-shareable", input)
	if code != 201 {
		t.Fatal(code, private)
	}
	privatePath := "/v1/owner/intents/" + private["id"].(string) + "/grants"
	code, _ = ownerCallMatch(owner, "POST", privatePath, "private-cannot-grant", private["etag"].(string), valid(time.Now().Add(time.Hour), []string{"objective"}))
	if code != 409 {
		t.Fatalf("private intent was promoted by grant command: %d", code)
	}

	input.Visibility = "restricted"
	code, source := owner.call("POST", "/v1/intents", "create-bounded-grant-source", input)
	if code != 201 {
		t.Fatal(code, source)
	}
	grantPath := "/v1/owner/intents/" + source["id"].(string) + "/grants"
	grantBody := valid(input.ExpiresAt.Add(-time.Minute), []string{"objective"})
	code, grant := ownerCallMatch(owner, "POST", grantPath, "create-bounded-grant", source["etag"].(string), grantBody)
	if code != 201 {
		t.Fatalf("bounded grant %d %#v", code, grant)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+source["id"].(string), "", nil)
	if code != 200 {
		t.Fatalf("live grant hidden: %d", code)
	}
	noReadOwner := client(t, s)
	code, noReadAgent := noReadOwner.call("POST", "/v1/agents", "create-agent-without-intent-read", map[string]any{"name": "No intent read", "scopes": []string{"inbox:read"}})
	if code != 201 {
		t.Fatalf("no-read agent %d %#v", code, noReadAgent)
	}
	noRead := testClient{t: t, s: s, owner: noReadOwner.owner, token: noReadAgent["token"].(string)}
	_, noReadIdentity := noRead.call("GET", "/v1/me", "", nil)
	noReadGrant := valid(input.ExpiresAt.Add(-time.Minute), []string{"objective"})
	noReadGrant["principal_id"] = noReadIdentity["principal_id"]
	code, noReadGrantResult := ownerCallMatch(owner, "POST", grantPath, "grant-without-read-scope", grant["etag"].(string), noReadGrant)
	if code != 201 {
		t.Fatalf("no-read grant %d %#v", code, noReadGrantResult)
	}
	code, _ = noRead.call("GET", "/v1/intents/"+source["id"].(string), "", nil)
	if code != 403 {
		t.Fatalf("grant bypassed credential scope: %d", code)
	}
	// Pausing the source hides the selected fields immediately. Resuming the
	// owner-controlled source does not create or widen a grant.
	code, paused := ownerCallMatch(owner, "PATCH", "/v1/owner/intents/"+source["id"].(string), "pause-grant-source", noReadGrantResult["etag"].(string), map[string]any{"state": "paused"})
	if code != 200 {
		t.Fatalf("pause source %d %#v", code, paused)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+source["id"].(string), "", nil)
	if code != 404 {
		t.Fatalf("paused source still readable: %d", code)
	}
	code, _ = ownerCallMatch(owner, "PATCH", "/v1/owner/intents/"+source["id"].(string), "resume-grant-source", paused["etag"].(string), map[string]any{"state": "active"})
	if code != 200 {
		t.Fatalf("resume source %d", code)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+source["id"].(string), "", nil)
	if code != 200 {
		t.Fatalf("source resume changed grant authority: %d", code)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.intent_grants SET expires_at=now()-interval '1 second' WHERE intent_id=$1", source["id"]); err != nil {
		t.Fatal(err)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+source["id"].(string), "", nil)
	if code != 404 {
		t.Fatalf("expired grant still readable: %d", code)
	}

	code, expiringSource := owner.call("POST", "/v1/intents", "create-expired-grant-source", input)
	if code != 201 {
		t.Fatal(code, expiringSource)
	}
	expiringPath := "/v1/owner/intents/" + expiringSource["id"].(string) + "/grants"
	code, _ = ownerCallMatch(owner, "POST", expiringPath, "grant-expiring-source", expiringSource["etag"].(string), valid(input.ExpiresAt.Add(-time.Minute), []string{"objective"}))
	if code != 201 {
		t.Fatal("grant source that will expire", code)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.intents SET expires_at=now()-interval '1 second' WHERE id=$1", expiringSource["id"]); err != nil {
		t.Fatal(err)
	}
	code, _ = recipient.call("GET", "/v1/intents/"+expiringSource["id"].(string), "", nil)
	if code != 404 {
		t.Fatalf("source expiry did not hide a still-live grant: %d", code)
	}
}

func TestRestrictedGrantReadRacingRevokeRechecksAfterCommit(t *testing.T) {
	s, _ := fixture(t)
	owner, recipient := client(t, s), client(t, s)
	input := sharingIntent()
	code, created := owner.call("POST", "/v1/intents", "create-revoke-race", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	_, target := recipient.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/intents/" + created["id"].(string) + "/grants"
	grant := map[string]any{"principal_id": target["principal_id"], "fields": []string{"objective"}, "expires_at": input.ExpiresAt.Add(-time.Minute)}
	code, granted := ownerCallMatch(owner, "POST", path, "create-revoke-race-grant", created["etag"].(string), grant)
	if code != 201 {
		t.Fatalf("grant %d %#v", code, granted)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var readCode, revokeCode int
	var readResult, revokeResult map[string]any
	go func() {
		defer wg.Done()
		<-start
		readCode, readResult = recipient.call("GET", "/v1/intents/"+created["id"].(string), "", nil)
	}()
	go func() {
		defer wg.Done()
		<-start
		revokeCode, revokeResult = ownerCallMatch(owner, "POST", path+"/"+target["principal_id"].(string)+"/revoke", "revoke-racing-grant", granted["etag"].(string), map[string]any{})
	}()
	close(start)
	wg.Wait()
	if revokeCode != 200 {
		t.Fatalf("concurrent revoke %d %#v", revokeCode, revokeResult)
	}
	if readCode != 200 && readCode != 404 {
		t.Fatalf("concurrent read had unexpected result %d %#v", readCode, readResult)
	}
	if readCode == 200 && (readResult["objective"] != input.Objective || readResult["budget"] != nil) {
		t.Fatalf("concurrent read violated selected field mask: %#v", readResult)
	}
	readCode, _ = recipient.call("GET", "/v1/intents/"+created["id"].(string), "", nil)
	if readCode != 404 {
		t.Fatalf("fresh read after committed revoke was not reauthorized: %d", readCode)
	}
}
