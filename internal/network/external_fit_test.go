package network

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
