package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func revisionInput(objective string) IntentInput {
	in := intentFixture()
	in.Objective = objective
	in.Preferences = map[string]string{"region": "EU"}
	return in
}

func revisionCall(c testClient, method, path, key, match string, body any) (int, map[string]any) {
	c.t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(canonical(body)))
	r.Header.Set("X-Test-Owner", c.owner)
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("If-Match", match)
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		c.t.Fatal(err)
	}
	return w.Code, out
}

func TestIntentionRevisionIsImmutableAndOwnerCommandSharesDomainPath(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	createdInput := revisionInput("Initial catalog request")
	code, created := a.call("POST", "/v1/intents", "create-revision", createdInput)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	firstETag := created["etag"].(string)
	firstDocument := canonical(createdInput)

	updated := revisionInput("Revised catalog request")
	revisionBody := map[string]any{"revision": updated}
	code, changed := revisionCall(a, "PATCH", "/v1/intents/"+id, "edit-revision", firstETag, revisionBody)
	if code != 200 || changed["objective"] != updated.Objective || changed["state"] != "active" || changed["version"] != float64(2) {
		t.Fatalf("edit %d %#v", code, changed)
	}
	code, replay := revisionCall(a, "PATCH", "/v1/intents/"+id, "edit-revision", firstETag, revisionBody)
	if code != 200 || replay["etag"] != changed["etag"] {
		t.Fatalf("revision retry changed result: %d %#v", code, replay)
	}
	code, _ = revisionCall(a, "PATCH", "/v1/intents/"+id, "stale-edit", firstETag, map[string]any{"revision": revisionInput("stale")})
	if code != 409 {
		t.Fatalf("stale edit accepted: %d", code)
	}
	restarted := a
	restarted.s = New(a.s.dsn, "development", nil)
	code, recovered := restarted.call("GET", "/v1/intents/"+id, "", nil)
	if code != 200 || recovered["objective"] != updated.Objective || recovered["etag"] != changed["etag"] {
		t.Fatalf("fresh handler did not recover acknowledged revision: %d %#v", code, recovered)
	}
	foreign := revisionInput("foreign")
	foreign.Output.SchemaFamily = "unsupported"
	code, _ = revisionCall(b, "PATCH", "/v1/intents/"+id, "foreign-edit", changed["etag"].(string), map[string]any{"revision": foreign})
	if code != 404 {
		t.Fatalf("foreign edit disclosed intent: %d", code)
	}
	invalid := revisionInput("bad")
	invalid.Output.SchemaFamily = "unsupported"
	code, _ = revisionCall(a, "PATCH", "/v1/intents/"+id, "invalid-edit", changed["etag"].(string), map[string]any{"revision": invalid})
	if code != 422 {
		t.Fatalf("invalid edit accepted: %d", code)
	}
	visibilityChange := revisionInput("Change disclosure without consent")
	visibilityChange.Visibility = "redacted_public"
	visibilityChange.PublicSummary = "new public disclosure"
	code, _ = revisionCall(a, "PATCH", "/v1/intents/"+id, "change-disclosure", changed["etag"].(string), map[string]any{"revision": visibilityChange})
	if code != 409 {
		t.Fatalf("revision silently changed disclosure visibility: %d", code)
	}

	var oldDocument, currentDocument, projection []byte
	var oldHash, currentHash string
	var currentRevision int
	var version int64
	var state string
	if err := db.QueryRow(context.Background(), `SELECT old.document,old.content_hash,current.document,current.public_projection,current.content_hash,i.current_revision,i.version,i.state FROM undergrid.intents i JOIN undergrid.intent_revisions old ON old.intent_id=i.id AND old.revision=1 JOIN undergrid.intent_revisions current ON current.intent_id=i.id AND current.revision=i.current_revision WHERE i.id=$1`, id).Scan(&oldDocument, &oldHash, &currentDocument, &projection, &currentHash, &currentRevision, &version, &state); err != nil {
		t.Fatal(err)
	}
	var decodedOld, decodedCurrent IntentInput
	if err := json.Unmarshal(oldDocument, &decodedOld); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(currentDocument, &decodedCurrent); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical(decodedOld), firstDocument) || oldHash != digest(firstDocument) {
		t.Fatalf("revision 1 mutated: document=%s hash=%s", oldDocument, oldHash)
	}
	var decodedProjection map[string]any
	if err := json.Unmarshal(projection, &decodedProjection); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical(decodedCurrent), canonical(updated)) || currentHash != digest(canonical(updated)) || len(decodedProjection) != 0 || currentRevision != 2 || version != 2 || state != "active" {
		t.Fatalf("current revision mismatch: doc=%s projection=%s revision=%d version=%d state=%s", currentDocument, projection, currentRevision, version, state)
	}
	var revisions, events, inbox int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1", id).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1", id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.aggregate_id=$1", id).Scan(&inbox); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || events != 2 || inbox != 2 {
		t.Fatalf("effects after retries/denials: revisions=%d events=%d inbox=%d", revisions, events, inbox)
	}
	var causeJobs int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND args->>'cause_id'=$1", id).Scan(&causeJobs); err != nil {
		t.Fatal(err)
	}
	if causeJobs != 2 {
		t.Fatalf("revision replay/denials changed durable scan jobs: %d", causeJobs)
	}

	// The owner HTTP adapter must reach the same append-only command and preserve
	// the existing active state, rather than implement a second revision policy.
	ownerUpdated := revisionInput("Owner-edited catalog request")
	code, ownerEdit := ownerCallMatch(a, "PATCH", "/v1/owner/intents/"+id, "owner-edit-revision", changed["etag"].(string), map[string]any{"revision": ownerUpdated})
	if code != 200 || ownerEdit["objective"] != ownerUpdated.Objective || ownerEdit["state"] != "active" {
		t.Fatalf("owner edit %d %#v", code, ownerEdit)
	}
	code, paused := ownerCallMatch(a, "PATCH", "/v1/owner/intents/"+id, "pause-for-edit", ownerEdit["etag"].(string), map[string]any{"state": "paused"})
	if code != 200 || paused["state"] != "paused" {
		t.Fatalf("pause before edit %d %#v", code, paused)
	}
	pausedInput := revisionInput("Paused catalog request update")
	code, stillPaused := ownerCallMatch(a, "PATCH", "/v1/owner/intents/"+id, "edit-while-paused", paused["etag"].(string), map[string]any{"revision": pausedInput})
	if code != 200 || stillPaused["state"] != "paused" || stillPaused["objective"] != pausedInput.Objective {
		t.Fatalf("paused edit changed state %d %#v", code, stillPaused)
	}
}

func TestExpiredIntentionCanBeRenewedByAppendingRevision(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	// Create validates future expiry, so establish a valid row and then let the
	// actual worker observe its deadline rather than writing the terminal state.
	initial := revisionInput("Expiring catalog request")
	initial.ExpiresAt = time.Now().Add(time.Hour)
	code, created := a.call("POST", "/v1/intents", "create-expiring", initial)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.intents SET expires_at=now()-interval '1 second',next_scan=now() WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := s.ScanDueIntentions(context.Background()); err != nil {
		t.Fatal(err)
	}
	code, expired := a.call("GET", "/v1/intents/"+id, "", nil)
	if code != 200 || expired["state"] != "expired" || expired["version"] != float64(2) {
		t.Fatalf("worker expiry %d %#v", code, expired)
	}

	renewed := revisionInput("Renewed catalog request")
	renewed.ExpiresAt = time.Now().Add(48 * time.Hour)
	code, result := revisionCall(a, "PATCH", "/v1/intents/"+id, "renew-expired", expired["etag"].(string), map[string]any{"revision": renewed})
	if code != 200 || result["objective"] != renewed.Objective || result["state"] != "active" || result["version"] != float64(3) {
		t.Fatalf("renewal %d %#v", code, result)
	}
	code, replay := revisionCall(a, "PATCH", "/v1/intents/"+id, "renew-expired", expired["etag"].(string), map[string]any{"revision": renewed})
	if code != 200 || replay["etag"] != result["etag"] {
		t.Fatalf("renewal retry changed result: %d %#v", code, replay)
	}
	var revision int
	var expires time.Time
	if err := db.QueryRow(context.Background(), "SELECT i.current_revision,i.expires_at FROM undergrid.intents i WHERE i.id=$1", id).Scan(&revision, &expires); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || !expires.Equal(renewed.ExpiresAt.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("renewal persistence revision=%d expires=%s expected=%s", revision, expires, renewed.ExpiresAt)
	}
	var events, inbox, scanJobs int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1", id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.aggregate_id=$1", id).Scan(&inbox); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND args->>'cause_id'=$1", id).Scan(&scanJobs); err != nil {
		t.Fatal(err)
	}
	if events != 3 || inbox != 3 || scanJobs != 2 {
		t.Fatalf("renewal replay effects: events=%d inbox=%d scan jobs=%d", events, inbox, scanJobs)
	}
}
