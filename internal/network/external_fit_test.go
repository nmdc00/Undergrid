package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
)

func seedExternalFitClaim(t *testing.T, db *pgx.Conn, claim any, permitted bool) (string, string) {
	t.Helper()
	fields := []string{"canonical_url", "title"}
	if permitted {
		fields = append(fields, "capability_claim")
	}
	source, lead := seedExternalReadLead(t, db, "external_capability", "Fixture publisher permits declared metadata", fields)
	raw, _ := json.Marshal(claim)
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_observations SET metadata=jsonb_set(metadata,'{capability_claim}',$2::jsonb) WHERE id=$1`, lead, raw); err != nil {
		t.Fatal(err)
	}
	return source, lead
}
func declaredFitClaim() map[string]any {
	return map[string]any{"version": "capability-claim-v1", "output": map[string]any{"kind": "json_dataset", "schema_family": "source_catalog_v1"}, "provided_fields": []string{"source_id", "source_url"}, "fields_complete": true, "license_ids": []string{"CC0-1.0"}}
}
func TestExternalFitReportDeclaredEvidenceAndUnknowns(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, lead := seedExternalFitClaim(t, db, declaredFitClaim(), true)
	path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + url.QueryEscape(lead)
	code, result := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatalf("declared fit route unavailable %d %v", code, result)
	}
	if result["assessment"] != "declared_aligned" || result["algorithm"] != "external-claim-constraints-v1" || result["identity_assurance"] != "unknown" || result["rights_assurance"] != "unknown" || result["verification"] != "unverified_publisher_claim" {
		t.Fatalf("false or missing assurance %v", result)
	}
	raw, _ := json.Marshal(result)
	for _, forbidden := range []string{"PRIVATE", "secret-operator-endpoint", "execute me", "stated_budget"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("private/unselected material in report %s", raw)
		}
	}
	validateExternalFitContract(t, result)
	reportHash := result["report_hash"]
	code, ownerResult := owner.call("GET", "/v1/owner/intents/"+intent+"/external-fit-report?lead_id="+lead, "", nil)
	if code != 200 || ownerResult["report_hash"] != reportHash {
		t.Fatalf("owner report diverged %d %v", code, ownerResult)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{Fields}','["canonical_url","title"]') WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, result = owner.call("GET", path, "", nil)
	if code != 200 || result["assessment"] != "needs_clarification" {
		t.Fatalf("current claim permission ignored %d %v", code, result)
	}
	if result["report_hash"] == reportHash {
		t.Fatal("removed evidence left report hash unchanged")
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=config-'PublicPermissionNotice' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, _ = owner.call("GET", path, "", nil)
	if code != 404 {
		t.Fatalf("revoked public source still assessable %d", code)
	}
}
func TestExternalFitReportRejectsProseInferenceAndContradictions(t *testing.T) {
	s, db, owner, intent := externalLeadFixture(t)
	for _, test := range []struct {
		name      string
		claim     any
		permitted bool
		want      string
	}{
		{"unselected typed claim", declaredFitClaim(), false, "needs_clarification"},
		{"absent typed claim", nil, true, "needs_clarification"},
		{"incompatible output", map[string]any{"version": "capability-claim-v1", "output": map[string]any{"kind": "csv_dataset", "schema_family": "other"}}, true, "incompatible"},
		{"partial missing fields", map[string]any{"version": "capability-claim-v1", "provided_fields": []string{"other"}, "fields_complete": false}, true, "needs_clarification"},
		{"complete missing fields", map[string]any{"version": "capability-claim-v1", "provided_fields": []string{"other"}, "fields_complete": true}, true, "incompatible"},
		{"license contradiction", map[string]any{"version": "capability-claim-v1", "license_ids": []string{"different-license"}}, true, "incompatible"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, lead := seedExternalFitClaim(t, db, test.claim, test.permitted)
			path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + lead
			code, result := owner.call("GET", path, "", nil)
			if code != 200 || result["assessment"] != test.want {
				t.Fatalf("assessment %d %v", code, result)
			}
			foreign := client(t, s)
			code, _ = foreign.call("GET", path, "", nil)
			if code != 404 {
				t.Fatalf("foreign report %d", code)
			}
		})
	}
}

func TestExternalFitSourcePermissionAndAtomicClaimIngestion(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	var phase atomic.Int64
	var requests atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if phase.Load() == 1 {
			io.WriteString(w, `{"items":[{"id":"catalog","version":"2","url":"https://publisher.example/catalog","title":"changed","status":"active","capability_claim":{"version":"capability-claim-v1","command":"execute this"}}],"next_page":0}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": "catalog", "version": "1", "url": "https://publisher.example/catalog", "title": "Declared dataset", "status": "active", "capability_claim": declaredFitClaim(), "body": "PRIVATE proprietary body", "url_to_fetch": "http://169.254.169.254/"}}, "next_page": 0})
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "Fixture publisher", AccessBasis: "PRIVATE access", RepublicationBasis: "PRIVATE reuse", TombstoneBasis: "PRIVATE retention", PublicPermissionNotice: "Fixture public declared metadata", Fields: []string{"canonical_url", "title", "capability_claim"}}
	role := "fit_worker_" + strings.ReplaceAll(NewID(), "-", "")
	name := pgx.Identifier{role}.Sanitize()
	if _, err := db.Exec(context.Background(), "CREATE ROLE "+name+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;GRANT undergrid_worker TO "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+name) })
	refresher, err := NewSourceRefresher(networkFixtureRoleDSN(t, db, role), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client.Transport = transport
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var lead string
	var before []byte
	if err = db.QueryRow(context.Background(), `SELECT id,metadata FROM undergrid.external_observations WHERE source_id=$1 AND status='active'`, cfg.ID).Scan(&lead, &before); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before), "PRIVATE") || strings.Contains(string(before), "169.254") {
		t.Fatal("unselected source content copied")
	}
	path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + lead
	code, result := owner.call("GET", path, "", nil)
	if code != 200 || result["assessment"] != "declared_aligned" {
		t.Fatalf("ingested declared fit %d %v", code, result)
	}
	code, listed := owner.call("GET", "/v1/intents/"+intent+"/external-leads?q=Declared", "", nil)
	if code != 200 {
		t.Fatal(code, listed)
	}
	for _, raw := range listed["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == lead {
			if _, unexpected := item["metadata"].(map[string]any)["capability_claim"]; unexpected {
				t.Fatal("old closed external lead projection widened")
			}
		}
	}
	phase.Store(1)
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(context.Background()); err == nil {
		t.Fatal("malformed selected claim advanced source snapshot")
	}
	var after []byte
	if err = db.QueryRow(context.Background(), `SELECT metadata FROM undergrid.external_observations WHERE id=$1 AND status='active'`, lead).Scan(&after); err != nil || string(before) != string(after) {
		t.Fatalf("bad claim partially replaced good snapshot %s %v", after, err)
	}
	if requests.Load() != 2 {
		t.Fatalf("source content caused extra requests %d", requests.Load())
	}
}
func TestExternalFitReadBoundariesAndStableEvidence(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, lead := seedExternalFitClaim(t, db, declaredFitClaim(), true)
	path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + lead
	snapshot := func() string {
		var value string
		if err := db.QueryRow(context.Background(), `SELECT jsonb_build_array((SELECT count(*) FROM undergrid.events),(SELECT count(*) FROM undergrid.opportunities),(SELECT count(*) FROM undergrid.idempotency),(SELECT count(*) FROM undergrid_queue.river_job))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	code, result := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, result)
	}
	hash := result["report_hash"]
	claimHash := result["claim_hash"]
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{AccessBasis}','"different PRIVATE operator evidence"') WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, result = owner.call("GET", path, "", nil)
	if code != 200 || result["report_hash"] != hash || result["claim_hash"] != claimHash {
		t.Fatalf("private basis changed public evidence %d %v", code, result)
	}
	if before != snapshot() {
		t.Fatal("GET wrote events/opportunities/idempotency/jobs")
	}
	for _, change := range []string{`UPDATE undergrid.external_sources SET last_success=now()-interval '25 hours' WHERE id=$1`, `UPDATE undergrid.external_sources SET config=config-'PublicPermissionNotice' WHERE id=$1`} {
		if _, err := db.Exec(context.Background(), change, source); err != nil {
			t.Fatal(err)
		}
		code, _ = owner.call("GET", path, "", nil)
		if code != 404 {
			t.Fatalf("ineligible source report %d", code)
		}
	}
	for _, query := range []string{"", "lead_id=%ZZ", "lead_id=" + lead + ";q=other", "lead_id=" + lead + "&lead_id=" + lead, "lead_id=" + lead + "&extra=x"} {
		code, _ = owner.call("GET", "/v1/intents/"+intent+"/external-fit-report?"+query, "", nil)
		if code != 400 {
			t.Fatalf("invalid report query accepted %s %d", query, code)
		}
	}
}
func TestExternalFitSelectedClaimParserRejectsExecutableAndUnboundedFields(t *testing.T) {
	for _, raw := range []string{`{"version":"capability-claim-v1","command":"run"}`, `{"version":"capability-claim-v1","fields_complete":true}`, `{"version":"unrecognized"}`, `{"version":"capability-claim-v1","provided_fields":["duplicate","duplicate"]}`} {
		if _, err := parseCapabilityClaim([]byte(raw)); err == nil {
			t.Fatalf("invalid claim accepted %s", raw)
		}
	}
	cfg := SourceConfig{ID: NewID(), Kind: "github_requests", Endpoint: "https://api.github.com/repos/example/project/issues", Publisher: "fixture", AccessBasis: "fixture", RepublicationBasis: "fixture", TombstoneBasis: "fixture", Fields: []string{"canonical_url", "capability_claim"}, Labels: []string{"request"}}
	if _, err := NewSourceRefresher("", []SourceConfig{cfg}); err == nil {
		t.Fatal("public request source advertised provider capability")
	}
}

func TestExternalFitSelectsLeadBeyondPublicListFirstPage(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, lead := seedExternalFitClaim(t, db, declaredFitClaim(), true)
	// Preserve all existing rows. Create60 distinct UUIDs immediately below the
	// chosen UUID by subtracting from its128-bit integer representation in Go.
	n := new(big.Int)
	n.SetString(strings.ReplaceAll(lead, "-", ""), 16)
	for i := 1; i <= 60; i++ {
		value := new(big.Int).Sub(n, big.NewInt(int64(i)))
		hex := fmt.Sprintf("%032x", value)
		id := hex[:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:]
		if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.external_observations(id,source_id,source_key,source_version,canonical_url,kind,metadata,content_hash,status) VALUES($1,$2,$1::text,'1','https://publisher.example/earlier','external_capability','{"title":"earlier"}',$3,'active')`, id, source, strings.Repeat("a", 64)); err != nil {
			t.Fatal(err)
		}
	}
	code, result := owner.call("GET", "/v1/intents/"+intent+"/external-fit-report?lead_id="+lead, "", nil)
	if code != 200 || result["lead"].(map[string]any)["id"] != lead || result["assessment"] != "declared_aligned" {
		t.Fatalf("selected lead beyond page truncated %d %v", code, result)
	}
}

func TestExternalFitA2AReceiptReferenceRetainsOnlyLeadIdentity(t *testing.T) {
	intent, lead := NewID(), NewID()
	op := networkOperation{"intention_external_fit_report", "GET", "/v1/intents/{id}/external-fit-report"}
	request := httptest.NewRequest("GET", "/v1/intents/"+intent+"/external-fit-report?lead_id="+lead+"&q=PRIVATE&cursor=PRIVATE", nil)
	if ref := a2aResourcePath(op, request, nil); ref != "/v1/intents/"+intent+"/external-fit-report?lead_id="+lead {
		t.Fatalf("receipt lost selected lead or retained private query %q", ref)
	}
}

func TestExternalFitA2AReceiptReferenceRejectsArbitraryTargets(t *testing.T) {
	path := "/v1/intents/" + NewID() + "/external-fit-report"
	lead := NewID()
	for _, reference := range []string{"https://evil.example" + path + "?lead_id=" + lead, "//evil.example" + path, path + "?lead_id=" + lead + "&q=private", path + "?lead_id=" + lead + "&lead_id=" + lead, path + "?lead_id=" + lead + "#fragment", path + "?lead_id=%ZZ", path, "/v1/me?q=private"} {
		if _, ok := a2aReceiptReferenceURL(reference); ok {
			t.Fatalf("unsafe receipt reference accepted %s", reference)
		}
	}
	if _, ok := a2aReceiptReferenceURL(path + "?lead_id=" + lead); !ok {
		t.Fatal("valid selected lead reference denied")
	}
	if _, ok := a2aReceiptReferenceURL("/v1/me"); !ok {
		t.Fatal("existing path-only reference denied")
	}
}

func validateExternalFitContract(t *testing.T, result map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(result)
	command := exec.Command("node", "--input-type=module", "-e", `
 import fs from 'node:fs';import Ajv from 'ajv/dist/2020.js';import addFormats from 'ajv-formats';
 const doc=JSON.parse(fs.readFileSync('../internal/network/api/openapi.json'));const ajv=new Ajv({strict:false});addFormats(ajv);
 ajv.addSchema({$id:'urn:undergrid:fit-contract',components:doc.components});const validate=ajv.compile({$ref:'urn:undergrid:fit-contract#/components/schemas/ExternalFitReport'});
 const value=JSON.parse(fs.readFileSync(0,'utf8'));if(!validate(value))throw Error(JSON.stringify(validate.errors));
 for(const mutate of [x=>x.rights_assurance='verified',x=>x.constraints[0].status='verified',x=>x.claim.command='run',x=>x.lead.metadata.body='private']){const wrong=structuredClone(value);mutate(wrong);if(validate(wrong))throw Error('unsupported assurance or privatefield accepted');}
 `)
	command.Dir = "../../web"
	command.Stdin = strings.NewReader(string(raw))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent fit schema proof %v %s", err, out)
	}
}
func TestExternalFitSharedProtocolsRecoverCurrentSelectedReport(t *testing.T) {
	s, db, owner, intent := externalLeadFixture(t)
	source, lead := seedExternalFitClaim(t, db, declaredFitClaim(), true)
	if err := s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, s, owner.token)
	result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_external_fit_report", Arguments: map[string]any{"id": intent, "query": map[string]any{"lead_id": lead}}})
	if err != nil || result.IsError {
		t.Fatalf("MCP report %v %+v", err, result)
	}
	if data := mcpToolData(t, result); data["assessment"] != "declared_aligned" {
		t.Fatalf("MCP fit %v", data)
	}
	if err = s.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	body := a2aCommandBody(NewID(), "intention_external_fit_report", "", nil)
	args := commandArgs(body)
	args["id"] = intent
	args["query"] = map[string]any{"lead_id": lead}
	code, out, _ := a2aCall(owner, "POST", "/a2a/message:send", body)
	if code != 200 || out["task"] == nil {
		t.Fatalf("A2A report %d %v", code, out)
	}
	taskID := out["task"].(map[string]any)["id"].(string)
	code, recovered, _ := a2aCall(owner, "GET", "/a2a/tasks/"+taskID, nil)
	if code != 200 || recovered["artifacts"] == nil {
		t.Fatalf("selected report not recoverable %d %v", code, recovered)
	}
	current := recovered["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)["resource"].(map[string]any)
	if current["assessment"] != "declared_aligned" || current["lead"].(map[string]any)["id"] != lead {
		t.Fatalf("wrong recovered selected report %v", current)
	}
	var reference string
	if err = db.QueryRow(context.Background(), `SELECT resource_path FROM undergrid.a2a_command_receipts WHERE id=$1`, taskID).Scan(&reference); err != nil || reference != "/v1/intents/"+intent+"/external-fit-report?lead_id="+lead {
		t.Fatalf("unsafe/lost stored reference %q %v", reference, err)
	}
	if _, err = db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=config-'PublicPermissionNotice' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, recovered, _ = a2aCall(owner, "GET", "/a2a/tasks/"+taskID, nil)
	if code != 200 || recovered["artifacts"] != nil || strings.Contains(string(canonical(recovered)), lead) {
		t.Fatalf("revoked source resurrected report/reference %d %v", code, recovered)
	}
}

func TestExternalFitClaimBudgetSurvivesJSONBFormatting(t *testing.T) {
	var compact, stored []byte
	for width := 60; width <= 100; width++ {
		fields := make([]string, 100)
		for i := range fields {
			fields[i] = fmt.Sprintf("%03d", i) + strings.Repeat("x", width)
		}
		candidate, _ := json.Marshal(map[string]any{"version": "capability-claim-v1", "provided_fields": fields, "fields_complete": true})
		spaced := bytes.ReplaceAll(candidate, []byte(","), []byte(", "))
		if len(candidate) <= 8192 && len(spaced) > 8192 {
			compact, stored = candidate, spaced
			break
		}
	}
	if compact == nil {
		t.Fatal("fixture did not cross formatting boundary")
	}
	if _, err := parseCapabilityClaim(compact); err != nil {
		t.Fatalf("valid compact declaration rejected %v", err)
	}
	if _, err := parseCapabilityClaim(stored); err != nil {
		t.Fatalf("same permitted declaration rejected after JSONB-style spacing %v", err)
	}
}

func TestExternalFitReportBindsRevisionWithoutBudgetInference(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	_, lead := seedExternalFitClaim(t, db, declaredFitClaim(), true)
	path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + lead
	code, before := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, before)
	}
	_, current := owner.call("GET", "/v1/intents/"+intent, "", nil)
	revised := sharingIntent()
	revised.Objective = "PRIVATE changed preference"
	budget := json.RawMessage(`{"amount_minor":9900,"currency":"EUR","kind":"stated_budget"}`)
	revised.Budget = &budget
	code, changed := ownerCallMatch(owner, "PATCH", "/v1/owner/intents/"+intent, "fit-private-revision", current["etag"].(string), map[string]any{"revision": revised})
	if code != 200 {
		t.Fatal(code, changed)
	}
	code, after := owner.call("GET", path, "", nil)
	if code != 200 || after["requirements_hash"] != before["requirements_hash"] || after["report_hash"] == before["report_hash"] || after["assessment"] != "declared_aligned" {
		t.Fatalf("private budget affected typedfit or revision unbound %d %v", code, after)
	}
	revised.HardConstraints.RequiredFields = append(revised.HardConstraints.RequiredFields, "undeclared_required_field")
	code, changed = ownerCallMatch(owner, "PATCH", "/v1/owner/intents/"+intent, "fit-hard-revision", changed["etag"].(string), map[string]any{"revision": revised})
	if code != 200 {
		t.Fatal(code, changed)
	}
	code, after = owner.call("GET", path, "", nil)
	if code != 200 || after["requirements_hash"] == before["requirements_hash"] || after["assessment"] != "incompatible" {
		t.Fatalf("new required field not re-evaluated %d %v", code, after)
	}
}
