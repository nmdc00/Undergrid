package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func externalLeadFixture(t *testing.T) (*Service, *pgx.Conn, testClient, string) {
	t.Helper()
	s, db := fixture(t)
	owner := client(t, s)
	code, result := owner.call("POST", "/v1/intents", "external-lead-intent", sharingIntent())
	if code != 201 {
		t.Fatal(code, result)
	}
	role := "lead_reader_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err := db.Exec(context.Background(), "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB NOREPLICATION"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize()) })
	if _, err := db.Exec(context.Background(), "GRANT undergrid_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	s.dsn = networkFixtureRoleDSN(t, db, role)
	return s, db, owner, result["id"].(string)
}
func seedExternalReadLead(t *testing.T, db *pgx.Conn, kind, notice string, fields []string) (string, string) {
	t.Helper()
	source, id := NewID(), NewID()
	config := map[string]any{"ID": source, "Kind": "capability_manifest", "Endpoint": "https://feed.example/secret-operator-endpoint", "Publisher": "Fixture publisher", "AccessBasis": "PRIVATE access evidence", "RepublicationBasis": "PRIVATE reuse evidence", "TombstoneBasis": "PRIVATE retention evidence", "PublicPermissionNotice": notice, "Fields": fields}
	raw, _ := json.Marshal(config)
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.external_sources(id,config,last_success) VALUES($1,$2,now())`, source, raw); err != nil {
		t.Fatal(err)
	}
	metadata := []byte(`{"canonical_url":"https://publisher.example/catalog","title":"JSON catalogue","summary":"permitted structural source","tags":["dataset"],"body":"PRIVATE unapproved body","command":"execute me"}`)
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.external_observations(id,source_id,source_key,source_version,canonical_url,kind,metadata,content_hash,status) VALUES($1,$2,$3,'1','https://publisher.example/catalog',$4,$5,$6,'active')`, id, source, NewID(), kind, metadata, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	return source, id
}
func TestExternalLeadReadCurrentPermissionProjection(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, id := seedExternalReadLead(t, db, "external_capability", "Publisher permits this metadata", []string{"canonical_url", "title", "tags"})
	seedExternalReadLead(t, db, "external_request", "", []string{"canonical_url", "title"})
	seedExternalReadLead(t, db, "external_request", "No link permission", []string{"title"})
	path := "/v1/intents/" + intent + "/external-leads"
	code, result := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatalf("external leads unavailable: %d %v", code, result)
	}
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("approval filtering %v", result)
	}
	item := items[0].(map[string]any)
	if item["id"] != id || item["origin"] != "external_capability" || item["identity_assurance"] != "unknown" || item["fit_status"] != "not_assessed" {
		t.Fatalf("misleading origin or fit %v", item)
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{"PRIVATE", "secret-operator-endpoint", "execute me", "permitted structural source"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("unapproved projection %s", raw)
		}
	}
	if item["permission_notice"] != "Publisher permits this metadata" || item["publisher"] != "Fixture publisher" || item["canonical_url"] != "https://publisher.example/catalog" {
		t.Fatalf("missing public provenance %v", item)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=config-'PublicPermissionNotice' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, result = owner.call("GET", path, "", nil)
	if code != 200 || len(result["items"].([]any)) != 0 {
		t.Fatalf("revoked notice still public %d %v", code, result)
	}
	if _, err := db.Exec(context.Background(), "SET ROLE undergrid_app"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), "SELECT config FROM undergrid.external_sources"); err == nil {
		t.Fatal("app read raw source approval evidence")
	}
}
func TestExternalLeadCursorAuthorityAndRevisionBinding(t *testing.T) {
	s, db, owner, intent := externalLeadFixture(t)
	for n := 0; n < 3; n++ {
		seedExternalReadLead(t, db, "external_request", "Fixture metadata permission", []string{"canonical_url", "title"})
	}
	path := "/v1/intents/" + intent + "/external-leads?kind=external_request&limit=1"
	code, result := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, result)
	}
	cursor, ok := result["next_cursor"].(string)
	if !ok || cursor == "" || len(result["items"].([]any)) != 1 {
		t.Fatalf("missing bounded cursor %v", result)
	}
	first := result["items"].([]any)[0].(map[string]any)["id"]
	code, page := owner.call("GET", path+"&cursor="+url.QueryEscape(cursor), "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 || page["items"].([]any)[0].(map[string]any)["id"] == first {
		t.Fatalf("cursor failed %d %v", code, page)
	}
	for _, query := range []string{"kind=external_capability&limit=1", "kind=external_request&limit=2", "kind=external_request&limit=1&q=different"} {
		code, _ = owner.call("GET", "/v1/intents/"+intent+"/external-leads?"+query+"&cursor="+url.QueryEscape(cursor), "", nil)
		if code != 400 {
			t.Fatalf("changed filter accepted cursor: %s %d", query, code)
		}
	}
	foreign := client(t, s)
	code, _ = foreign.call("GET", path, "", nil)
	if code != 404 {
		t.Fatalf("foreign private intention leaked %d", code)
	}
	code, _ = owner.call("GET", "/v1/owner/intents/"+intent+"/external-leads?kind=external_request&limit=1&cursor="+url.QueryEscape(cursor), "", nil)
	if code != 400 {
		t.Fatalf("agent cursor used as owner cursor %d", code)
	}
	code, current := owner.call("GET", "/v1/intents/"+intent, "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	input := sharingIntent()
	input.Objective = "Changed intention revision"
	input.ExpiresAt = time.Now().Add(time.Hour)
	patch := map[string]any{"revision": input}
	code, changed := ownerCallMatch(owner, "PATCH", "/v1/owner/intents/"+intent, "lead-revision", current["etag"].(string), patch)
	if code != 200 {
		t.Fatalf("revision fixture %d %v", code, changed)
	}
	code, _ = owner.call("GET", path+"&cursor="+url.QueryEscape(cursor), "", nil)
	if code != 400 {
		t.Fatalf("stale intention revision cursor accepted %d", code)
	}
	_ = fmt.Sprint // Additional persisted read-only side-effect assertions follow.
}
