package network

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
	"os/exec"
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
func seedExternalReadLead(t *testing.T, db *pgx.Conn, kind, notice string, fields []string, markers ...string) (string, string) {
	t.Helper()
	source, id := NewID(), NewID()
	config := map[string]any{"ID": source, "Kind": "capability_manifest", "Endpoint": "https://feed.example/secret-operator-endpoint", "Publisher": "Fixture publisher", "AccessBasis": "PRIVATE access evidence", "RepublicationBasis": "PRIVATE reuse evidence", "TombstoneBasis": "PRIVATE retention evidence", "PublicPermissionNotice": notice, "Fields": fields}
	raw, _ := json.Marshal(config)
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.external_sources(id,config,last_success) VALUES($1,$2,now())`, source, raw); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(map[string]any{"canonical_url": "https://publisher.example/catalog", "title": "JSON catalogue " + source + " " + strings.Join(markers, " "), "summary": "permitted structural source", "tags": []string{"dataset"}, "body": "PRIVATE unapproved body", "command": "execute me"})
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.external_observations(id,source_id,source_key,source_version,canonical_url,kind,metadata,content_hash,status) VALUES($1,$2,$3,'1','https://publisher.example/catalog',$4,$5,$6,'active')`, id, source, NewID(), kind, metadata, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(context.Background(), `UPDATE undergrid.external_sources SET last_success=NULL WHERE id=$1`, source)
	})
	return source, id
}
func TestExternalLeadReadCurrentPermissionProjection(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, id := seedExternalReadLead(t, db, "external_capability", "Publisher permits this metadata", []string{"canonical_url", "title", "tags"})
	seedExternalReadLead(t, db, "external_request", "", []string{"canonical_url", "title"}, source)
	seedExternalReadLead(t, db, "external_request", "No link permission", []string{"title"}, source)
	path := "/v1/intents/" + intent + "/external-leads?q=" + url.QueryEscape(source)
	code, result := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatalf("external leads unavailable: %d %v", code, result)
	}
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("approval filtering %v", result)
	}
	item := items[0].(map[string]any)
	validateExternalLeadContract(t, result)
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
	code, ownerResult := owner.call("GET", "/v1/owner/intents/"+intent+"/external-leads?q="+url.QueryEscape(source), "", nil)
	if code != 200 || len(ownerResult["items"].([]any)) != 1 {
		t.Fatalf("owner lead read %d %v", code, ownerResult)
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
	defer db.Exec(context.Background(), "RESET ROLE")
	if _, err := db.Exec(context.Background(), "SELECT config FROM undergrid.external_sources"); err == nil {
		t.Fatal("app read raw source approval evidence")
	}
}
func TestExternalLeadCursorAuthorityAndRevisionBinding(t *testing.T) {
	s, db, owner, intent := externalLeadFixture(t)
	marker := NewID()
	for n := 0; n < 3; n++ {
		seedExternalReadLead(t, db, "external_request", "Fixture metadata permission", []string{"canonical_url", "title"}, marker)
	}
	path := "/v1/intents/" + intent + "/external-leads?kind=external_request&limit=1&q=" + url.QueryEscape(marker)
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
	code, _ = owner.call("GET", path+"&cursor="+url.QueryEscape(cursor+"x"), "", nil)
	if code != 400 {
		t.Fatalf("tampered cursor accepted %d", code)
	}
	var revision int
	var principal, agent string
	if err := db.QueryRow(context.Background(), `SELECT current_revision,principal_id,agent_id FROM undergrid.intents WHERE id=$1`, intent).Scan(&revision, &principal, &agent); err != nil {
		t.Fatal(err)
	}
	sibling := owner
	sibling.token = "ug_" + NewID()
	hash := sha256.Sum256([]byte(sibling.token))
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.credentials(id,agent_id,digest,scopes,audience,expires_at) VALUES($1,$2,$3,$4,'undergrid-network',now()+interval '1 hour')`, NewID(), agent, hash[:], []string{"intents:read"}); err != nil {
		t.Fatal(err)
	}
	code, _ = sibling.call("GET", path+"&cursor="+url.QueryEscape(cursor), "", nil)
	if code != 400 {
		t.Fatalf("sibling credential accepted foreign cursor %d", code)
	}
	blocked := owner
	blocked.token = "ug_" + NewID()
	hash = sha256.Sum256([]byte(blocked.token))
	if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.credentials(id,agent_id,digest,scopes,audience,expires_at) VALUES($1,$2,$3,$4,'undergrid-network',now()+interval '1 hour')`, NewID(), agent, hash[:], []string{"inbox:read"}); err != nil {
		t.Fatal(err)
	}
	code, _ = blocked.call("GET", path, "", nil)
	if code != 403 {
		t.Fatalf("missing intents:read scope accepted %d", code)
	}
	var credential string
	if err := db.QueryRow(context.Background(), `SELECT id FROM undergrid.credentials WHERE digest=$1`, sha256Bytes(owner.token)).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	expired := s.encodeExternalLeadCursor(externalLeadCursor{Purpose: "external-leads-v1", Environment: s.environment, Principal: principal, Agent: agent, Credential: credential, Channel: "agent", Intention: intent, Kind: "external_request", Query: marker, Revision: revision, Limit: 1, After: first.(string), Issued: time.Now().Add(-31 * time.Minute).Unix()})
	code, _ = owner.call("GET", path+"&cursor="+url.QueryEscape(expired), "", nil)
	if code != 410 {
		t.Fatalf("expired cursor accepted %d", code)
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
}

func TestExternalLeadQueryTimeEligibilityAndNoReadEffects(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, id := seedExternalReadLead(t, db, "external_capability", "Fixture public metadata permission", []string{"canonical_url", "title"})
	basePath := "/v1/intents/" + intent + "/external-leads"
	path := basePath + "?q=" + url.QueryEscape(source)
	counts := func() string {
		var result string
		err := db.QueryRow(context.Background(), `SELECT jsonb_build_array((SELECT count(*) FROM undergrid.opportunities),(SELECT count(*) FROM undergrid.events),(SELECT count(*) FROM undergrid.idempotency))::text`).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := counts()
	code, result := owner.call("GET", path, "", nil)
	if code != 200 || len(result["items"].([]any)) != 1 {
		t.Fatalf("eligible metadata missing %d %v", code, result)
	}
	hash := result["items"].([]any)[0].(map[string]any)["projection_hash"]
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{AccessBasis}','"different PRIVATE operator evidence"') WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, result = owner.call("GET", path, "", nil)
	if code != 200 || result["items"].([]any)[0].(map[string]any)["projection_hash"] != hash {
		t.Fatalf("private basis affected public digest %d %v", code, result)
	}
	code, result = owner.call("GET", basePath+"?q=PRIVATE", "", nil)
	if code != 200 || len(result["items"].([]any)) != 0 {
		t.Fatalf("filter searched unapproved evidence %d %v", code, result)
	}
	code, result = owner.call("GET", basePath+"?q=%25", "", nil)
	if code != 200 || len(result["items"].([]any)) != 0 {
		t.Fatalf("literal percent became SQL wildcard %d %v", code, result)
	}
	code, result = owner.call("GET", basePath+"?q="+url.QueryEscape(strings.Repeat("é", 100)), "", nil)
	if code != 200 {
		t.Fatalf("schema-valid Unicode query rejected %d %v", code, result)
	}
	if counts() != before {
		t.Fatal("GET appended domain/event/idempotency effects")
	}
	for _, test := range []struct{ name, change, restore string }{
		{"observation stale", `UPDATE undergrid.external_observations SET checked_at=now()-interval '25 hours' WHERE id=$1`, `UPDATE undergrid.external_observations SET checked_at=now() WHERE id=$1`},
		{"withdrawn", `UPDATE undergrid.external_observations SET status='closed' WHERE id=$1`, `UPDATE undergrid.external_observations SET status='active' WHERE id=$1`},
		{"source stale", `UPDATE undergrid.external_sources SET last_success=now()-interval '25 hours' WHERE id=$1`, `UPDATE undergrid.external_sources SET last_success=now() WHERE id=$1`},
		{"source removed", `UPDATE undergrid.external_sources SET last_success=NULL WHERE id=$1`, `UPDATE undergrid.external_sources SET last_success=now() WHERE id=$1`},
		{"link permission removed", `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{Fields}','["title"]') WHERE id=$1`, `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{Fields}','["canonical_url","title"]') WHERE id=$1`},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := id
			if strings.Contains(test.change, "external_sources") {
				key = source
			}
			if _, err := db.Exec(context.Background(), test.change, key); err != nil {
				t.Fatal(err)
			}
			defer db.Exec(context.Background(), test.restore, key)
			code, result := owner.call("GET", path, "", nil)
			if code != 200 || len(result["items"].([]any)) != 0 {
				t.Fatalf("ineligible lead still visible %d %v", code, result)
			}
		})
	}
	code, _ = owner.call("DELETE", "/v1/credentials/current", "", nil)
	if code != 200 {
		t.Fatalf("revoke fixture %d", code)
	}
	code, _ = owner.call("GET", path, "", nil)
	if code != 401 {
		t.Fatalf("revoked credential read leads %d", code)
	}
}

func sha256Bytes(value string) []byte { h := sha256.Sum256([]byte(value)); return h[:] }

func validateExternalLeadContract(t *testing.T, response map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(response)
	command := exec.Command("node", "--input-type=module", "-e", `
 import fs from 'node:fs';import Ajv from 'ajv/dist/2020.js';import addFormats from 'ajv-formats';
 const d=JSON.parse(fs.readFileSync('../internal/network/api/openapi.json'));
 const a=new Ajv({strict:false});addFormats(a);a.addSchema({$id:'urn:undergrid:external-leads',components:d.components});
 const validate=a.compile({$ref:'urn:undergrid:external-leads#/components/schemas/ExternalLeads'});
 const value=JSON.parse(fs.readFileSync(0,'utf8'));if(!validate(value))throw Error(JSON.stringify(validate.errors));
 for(const mutate of [x=>x.items[0].metadata.body='secret',x=>x.items[0].origin='registered_intent',x=>x.items[0].fit_status='verified']){const invalid=structuredClone(value);mutate(invalid);if(validate(invalid))throw Error('privacy/evidence negative oracle accepted');}
 `)
	command.Dir = "../../web"
	command.Stdin = strings.NewReader(string(raw))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent external lead schema proof: %v %s", err, out)
	}
}
func TestExternalLeadSharedMCPAndA2ARead(t *testing.T) {
	s, db, owner, intent := externalLeadFixture(t)
	source, id := seedExternalReadLead(t, db, "external_capability", "Fixture public permission", []string{"canonical_url", "title"})
	if err := s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, s, owner.token)
	result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_external_leads", Arguments: map[string]any{"id": intent, "query": map[string]any{"q": source, "limit": 1}}})
	if err != nil || result.IsError {
		t.Fatalf("MCP external read %v %+v", err, result)
	}
	data := mcpToolData(t, result)
	if len(data["items"].([]any)) != 1 || data["items"].([]any)[0].(map[string]any)["id"] != id {
		t.Fatalf("MCP external projection %v", data)
	}
	if err := s.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	body := a2aCommandBody(NewID(), "intention_external_leads", "", nil)
	args := commandArgs(body)
	args["id"] = intent
	args["query"] = map[string]any{"q": source, "limit": 1}
	code, out, _ := a2aCall(owner, "POST", "/a2a/message:send", body)
	if code != 200 || out["task"] == nil {
		t.Fatalf("A2A external read %d %v", code, out)
	}
	data = a2aLiveTaskResult(out)
	if len(data["items"].([]any)) != 1 || data["items"].([]any)[0].(map[string]any)["id"] != id {
		t.Fatalf("A2A external projection %v", data)
	}
}
