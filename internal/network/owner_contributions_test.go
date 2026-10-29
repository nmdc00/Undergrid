package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOwnerContributionCreatesSessionBoundQuarantineWithoutBorrowingCredential(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	data := []byte(`{"sources":[{"source_id":"owner-fixture","source_url":"https://example.org/"}]}`)
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)}
	key := NewID()
	code, upload := ownerCallMatch(owner, "POST", path, key, x.task["etag"].(string), body)
	if code != 201 {
		t.Fatalf("owner contribution unavailable after normal accepted plan: %d %v", code, upload)
	}
	if upload["state"] != "quarantine" || upload["purpose"] != "output" || upload["upload_authentication"] != "owner_session" {
		t.Fatal("missing explicit owner quarantine mode")
	}
	parsed, err := url.Parse(upload["upload_url"].(string))
	if err != nil || parsed.Path != "/v1/owner/agents/"+who["agent_id"].(string)+"/artifacts/"+upload["id"].(string)+"/upload" || len(parsed.Query().Get("token")) != 64 {
		t.Fatal("invalid owner upload capability route")
	}
	if code, replayed := ownerCallMatch(owner, "POST", path, key, x.task["etag"].(string), body); code != 201 || !reflect.DeepEqual(upload, replayed) {
		t.Fatal("owner quarantine creation did not recover exactly", code)
	}
	var mode, agent, principal, state string
	var noCredential bool
	var artifacts, approvals int
	if err := x.db.QueryRow(context.Background(), `SELECT upload_authentication,upload_credential_id IS NULL,agent_id,principal_id,(SELECT state FROM undergrid.tasks WHERE id=a.task_id),(SELECT count(*) FROM undergrid.artifacts WHERE task_id=a.task_id),(SELECT count(*) FROM undergrid.approvals WHERE plan_id=a.plan_id) FROM undergrid.artifacts a WHERE id=$1`, upload["id"]).Scan(&mode, &noCredential, &agent, &principal, &state, &artifacts, &approvals); err != nil {
		t.Fatal(err)
	}
	if mode != "owner_session" || !noCredential || agent != who["agent_id"] || principal != who["principal_id"] || state != "accepted" || artifacts != 1 || approvals != 6 {
		t.Fatal("owner upload borrowed authority or submitted work", mode, noCredential, agent, principal, state, artifacts, approvals)
	}
}

func TestOwnerContributionRecoveryCannotBorrowCredentialGeneration(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	data := []byte(`{"sources":[{"source_id":"separate","source_url":"https://example.org/"}]}`)
	key := NewID()
	legacy := x.upload(t, data, key)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": "output"}
	code, owned := ownerCallMatch(owner, "POST", path, key, x.task["etag"].(string), body)
	if code != 201 || owned["id"] == legacy["id"] || owned["upload_authentication"] != "owner_session" {
		t.Fatal("owner reused credential-bound generation", code)
	}
	agentBody := map[string]any{"swarm_id": x.room["id"], "plan_id": x.plan["id"], "task_id": x.task["id"], "claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": "output"}
	if code, replayed := owner.call("POST", "/v1/artifacts/uploads", key, agentBody); code != 201 || !reflect.DeepEqual(legacy, replayed) {
		t.Fatal("owner changed credential recovery", code)
	}
	parsed, err := url.Parse(owned["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// Owner capabilities cannot authenticate through the anonymous credential path.
	target := x.server.URL + "/v1/artifacts/" + owned["id"].(string) + "/upload?" + parsed.RawQuery
	request, err := http.NewRequest("PUT", target, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	response, err := x.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("owner capability authenticated without owner session", response.StatusCode)
	}
}

func TestOwnerContributionCreationRequiresCurrentTaskAndOwner(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": 12}
	for _, scenario := range []struct {
		name, match string
		actor       testClient
		body        map[string]any
		status      int
	}{
		{"missing task version", "", owner, body, 428},
		{"stale task version", "\"999\"", owner, body, 409},
		{"stale epoch", x.task["etag"].(string), owner, map[string]any{"claim_epoch": x.task["claim_epoch"].(float64) + 1, "media_type": "application/json", "size_bytes": 12}, 409},
		{"foreign owner", x.task["etag"].(string), x.actors[3], body, 404},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if code, _ := ownerCallMatch(scenario.actor, "POST", path, NewID(), scenario.match, scenario.body); code != scenario.status {
				t.Fatal("unexpected quarantine guard", code)
			}
		})
	}
	var count int
	if err := x.db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.artifacts WHERE task_id=$1`, x.task["id"]).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied upload allocated storage", count, err)
	}
}

func TestOwnerContributionDoesNotRequireLiveNetworkCredential(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	// Controlled credential revocation leaves the current owner and agent active.
	if _, err := x.db.Exec(context.Background(), `UPDATE undergrid.credentials SET revoked_at=clock_timestamp() WHERE agent_id=$1`, who["agent_id"]); err != nil {
		t.Fatal(err)
	}
	if code, _ := owner.call("GET", "/v1/me", "", nil); code != 401 {
		t.Fatal("revoked credential remained valid", code)
	}
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	code, upload := ownerCallMatch(owner, "POST", path, NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": 12})
	if code != 201 || upload["upload_authentication"] != "owner_session" {
		t.Fatal("owner improperly depended on revoked network credential", code)
	}
	for _, change := range []string{"upload_authentication='credential'", "generation=gen_random_uuid()", "upload_digest=decode(repeat('00',32),'hex')", "upload_expires_at=upload_expires_at+interval '1 minute'"} {
		_, err := x.db.Exec(context.Background(), "UPDATE undergrid.artifacts SET "+change+" WHERE id=$1", upload["id"])
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "23514" {
			t.Fatal("upload binding not protected by constraint", change, err)
		}
	}
}

func ownerContributionPut(c testClient, path string, data []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PUT", path, bytes.NewReader(data))
	r.Header.Set("X-Test-Owner", c.owner)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	return w
}

func TestOwnerContributionUploadsFinalizesAndReadsImmutableOutput(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	base := "/v1/owner/agents/" + who["agent_id"].(string)
	data := []byte(`{"sources":[{"source_id":"owner-output","source_url":"https://example.org/","instructions":"untrusted inert content"}]}`)
	code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
	if code != 201 {
		t.Fatal("normal owner quarantine failed", code)
	}
	target, err := url.Parse(upload["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 204 {
		t.Fatalf("owner byte upload unavailable: %d %s", response.Code, response.Body.String())
	}
	path := base + "/artifacts/" + upload["id"].(string)
	body := x.finalBody(data)
	key := NewID()
	code, sealed := ownerCallMatch(owner, "POST", path+"/finalize", key, "", body)
	if code != 200 || sealed["state"] != "sealed" || sealed["sha256"] != digest(data) {
		t.Fatal("owner immutable finalization failed", code, sealed)
	}
	if code, recovered := ownerCallMatch(owner, "POST", path+"/finalize", key, "", body); code != 200 || !reflect.DeepEqual(recovered, sealed) {
		t.Fatal("owner finalization did not recover exactly", code)
	}
	if code, current := ownerCallMatch(owner, "GET", path, "", "", nil); code != 200 || !reflect.DeepEqual(current, sealed) {
		t.Fatal("owner immutable readback failed", code)
	}
	if code, evaluations := ownerCallMatch(owner, "GET", path+"/evaluations", "", "", nil); code != 200 || len(evaluations["items"].([]any)) != 0 {
		t.Fatal("pending artifact fabricated verification", code, evaluations)
	}
	var state, sha, manifestHash, taskState, artifact string
	var size int
	var approvals, jobs int
	if err := x.db.QueryRow(context.Background(), `SELECT a.state,a.sha256,a.manifest_hash,a.size_bytes,t.state,t.submitted_artifact_id,(SELECT count(*) FROM undergrid.approvals WHERE plan_id=a.plan_id),(SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'artifact_id'=a.id::text) FROM undergrid.artifacts a JOIN undergrid.tasks t ON t.id=a.task_id WHERE a.id=$1`, upload["id"]).Scan(&state, &sha, &manifestHash, &size, &taskState, &artifact, &approvals, &jobs); err != nil {
		t.Fatal(err)
	}
	if state != "sealed" || sha != digest(data) || manifestHash != sealed["manifest_hash"] || size != len(data) || taskState != "submitted" || artifact != upload["id"] || approvals != 6 || jobs != 1 {
		t.Fatal("owner output not normalized or consent fabricated", state, sha, manifestHash, size, taskState, artifact, approvals, jobs)
	}
}

func TestOwnerContributionBytesRequireSessionCapabilityAndProducerMode(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	base := "/v1/owner/agents/" + who["agent_id"].(string)
	data := []byte(`{"sources":[{"source_id":"guarded","source_url":"https://example.org/"}]}`)
	code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
	if code != 201 {
		t.Fatal(code)
	}
	target, err := url.Parse(upload["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, path string
		actor      testClient
		status     int
	}{
		{"missing capability", target.Path, owner, 401},
		{"invalid capability", target.Path + "?token=" + strings.Repeat("z", 64), owner, 401},
		{"foreign owner selecting producer", target.RequestURI(), x.actors[3], 404},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if response := ownerContributionPut(scenario.actor, scenario.path, data); response.Code != scenario.status {
				t.Fatal("unauthorized owner bytes", response.Code)
			}
		})
	}
	_, coordinator := x.actors[0].call("GET", "/v1/me", "", nil)
	other := "/v1/owner/agents/" + coordinator["agent_id"].(string) + "/artifacts/" + upload["id"].(string) + "/upload?" + target.RawQuery
	if response := ownerContributionPut(x.actors[0], other, data); response.Code != 403 {
		t.Fatal("member inherited producer authority", response.Code)
	}
	if code, _ := owner.call("POST", "/v1/artifacts/"+upload["id"].(string)+"/finalize", NewID(), x.finalBody(data)); code != 403 {
		t.Fatal("network credential finalized owner generation", code)
	}
	if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 204 {
		t.Fatal("authorized owner byte transfer failed", response.Code)
	}
	stored, err := x.s.artifacts.read(upload["generation"].(string), int64(len(data)))
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatal("owner bytes not observed exactly", err)
	}
	legacy := x.upload(t, data, NewID())
	legacyURL, err := url.Parse(legacy["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	borrowed := base + "/artifacts/" + legacy["id"].(string) + "/upload?" + legacyURL.RawQuery
	if response := ownerContributionPut(owner, borrowed, data); response.Code != 403 {
		t.Fatal("owner session borrowed credential generation", response.Code)
	}
}

func TestOwnerContributionFixedEvaluationKeepsPurposeAndConsentSeparate(t *testing.T) {
	for _, purpose := range []string{"output", "progress"} {
		t.Run(purpose, func(t *testing.T) {
			x := newArtifactScenario(t)
			owner := x.actors[2]
			_, who := owner.call("GET", "/v1/me", "", nil)
			base := "/v1/owner/agents/" + who["agent_id"].(string)
			data := []byte(`{"sources":[{"source_id":"fixed-owner-check","source_url":"https://example.org/"}]}`)
			body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": purpose}
			if purpose == "progress" {
				body["audience_principal_ids"] = []string{x.principals[0], x.principals[2]}
				body["reviewer_principal_id"] = x.principals[0]
			}
			code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), body)
			if code != 201 {
				t.Fatal("owner purpose upload rejected", purpose, code)
			}
			target, err := url.Parse(upload["upload_url"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 204 {
				t.Fatal(response.Code)
			}
			path := base + "/artifacts/" + upload["id"].(string)
			if code, _ := ownerCallMatch(owner, "POST", path+"/finalize", NewID(), "", x.finalBody(data)); code != 200 {
				t.Fatal(code)
			}
			x.startWorker(t)
			x.waitEvaluation(t, upload["id"].(string))
			code, evaluations := ownerCallMatch(owner, "GET", path+"/evaluations", "", "", nil)
			if code != 200 {
				t.Fatal(code)
			}
			rows := evaluations["items"].([]any)
			if len(rows) != 1 {
				t.Fatal("fixed evaluation count", len(rows))
			}
			evaluation := rows[0].(map[string]any)
			if evaluation["result"] != "passed" || evaluation["input_sha256"] != digest(data) || evaluation["runtime_isolation"] != "development_process_not_qualified" || evaluation["verifier_sha256"] != x.config.VerifierSHA256 {
				t.Fatal("unqualified fixed worker evidence mismatch", evaluation)
			}
			_, artifact := ownerCallMatch(owner, "GET", path, "", "", nil)
			if artifact["state"] != "verified" || artifact["purpose"] != purpose {
				t.Fatal("owner artifact read lost purpose", artifact)
			}
			if purpose == "progress" {
				_, peer := x.actors[1].call("GET", "/v1/me", "", nil)
				for _, suffix := range []string{"", "/evaluations", "/content"} {
					if code, _ := ownerCallMatch(x.actors[1], "GET", "/v1/owner/agents/"+peer["agent_id"].(string)+"/artifacts/"+upload["id"].(string)+suffix, "", "", nil); code != 404 {
						t.Fatal("progress audience leaked through owner wrapper", suffix, code)
					}
				}
			}
			var taskState string
			var progress, cure *time.Time
			var credited *string
			var approvals, reports int
			if err := x.db.QueryRow(context.Background(), `SELECT state,progress_deadline,cure_deadline,credited_progress_evidence_id,(SELECT count(*) FROM undergrid.approvals WHERE plan_id=t.plan_id),(SELECT count(*) FROM undergrid.task_progress_evidence WHERE task_id=t.id) FROM undergrid.tasks t WHERE id=$1`, x.task["id"]).Scan(&taskState, &progress, &cure, &credited, &approvals, &reports); err != nil {
				t.Fatal(err)
			}
			want := "submitted"
			if purpose == "progress" {
				want = "accepted"
			}
			if taskState != want || credited != nil || approvals != 6 || reports != 0 {
				t.Fatal("evaluation fabricated acceptance or progress credit", taskState, credited, approvals, reports)
			}
			if purpose == "progress" {
				_, current := owner.call("GET", "/v1/tasks/"+x.task["id"].(string), "", nil)
				if !reflect.DeepEqual(current["progress_deadline"], x.task["progress_deadline"]) || !reflect.DeepEqual(current["cure_deadline"], x.task["cure_deadline"]) {
					t.Fatal("fixed evaluation renewed progress clocks")
				}
			}
		})
	}
}

func TestOwnerContributionExpiredCapabilityCannotOverwriteObservedBytes(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	base := "/v1/owner/agents/" + who["agent_id"].(string)
	data := []byte(`{"sources":[{"source_id":"expiry-a","source_url":"https://example.org/"}]}`)
	code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
	if code != 201 {
		t.Fatal(code)
	}
	target, err := url.Parse(upload["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 204 {
		t.Fatal(response.Code)
	}
	expiry, err := time.Parse(time.RFC3339Nano, upload["expires_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	timer := time.NewTimer(max(time.Until(expiry.Add(20*time.Millisecond)), 0))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("capability expiry observation timed out")
	}
	changed := bytes.Replace(data, []byte("expiry-a"), []byte("expiry-b"), 1)
	response := ownerContributionPut(owner, target.RequestURI(), changed)
	var failure map[string]any
	if err = json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if response.Code != 401 || failure["error"].(map[string]any)["code"] != "invalid_upload_capability" {
		t.Fatal("expired upload remained writable", response.Code)
	}
	stored, err := x.s.artifacts.read(upload["generation"].(string), int64(len(data)))
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatal("expired upload overwrote observed bytes", err)
	}
	// Capability expiry limits byte transfer, not owner authorization to seal
	// already uploaded exact bytes under current task/plan/policy authority.
	if code, out := ownerCallMatch(owner, "POST", base+"/artifacts/"+upload["id"].(string)+"/finalize", NewID(), "", x.finalBody(data)); code != 200 || out["sha256"] != digest(data) {
		t.Fatal("exact uploaded data could not be finalized after capability expiry", code)
	}
}

// A previously issued capability and retained finalization receipt never replace
// the producer's current contribution authority.
func TestOwnerContributionPolicyWithdrawalBlocksBytesAndReceiptRecovery(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		t.Run(map[bool]string{false: "quarantine", true: "sealed receipt"}[sealed], func(t *testing.T) {
			x := newArtifactScenario(t)
			owner := x.actors[2]
			_, who := owner.call("GET", "/v1/me", "", nil)
			base := "/v1/owner/agents/" + who["agent_id"].(string)
			data := []byte(`{"sources":[{"source_id":"withdrawal","source_url":"https://example.org/"}]}`)
			code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
			if code != 201 {
				t.Fatal(code, upload)
			}
			target, err := url.Parse(upload["upload_url"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 204 {
				t.Fatal(response.Code)
			}
			path := base + "/artifacts/" + upload["id"].(string) + "/finalize"
			key := NewID()
			body := x.finalBody(data)
			if sealed {
				if code, out := ownerCallMatch(owner, "POST", path, key, "", body); code != 200 {
					t.Fatal(code, out)
				}
				if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 409 {
					t.Fatal("sealed generation remained writable", response.Code)
				}
			}
			_, policy := owner.call("GET", "/v1/owner/policy", "", nil)
			if code, out := owner.versioned("POST", "/v1/owner/policy", NewID(), policy["etag"].(string), map[string]any{"template_ids": []string{}, "license_ids": []string{}, "task_categories": []string{}, "max_active_tasks": 5, "expires_at": time.Now().Add(time.Hour)}); code != 200 {
				t.Fatal(code, out)
			}
			if response := ownerContributionPut(owner, target.RequestURI(), data); response.Code != 403 {
				t.Fatal("issued capability bypassed current policy", response.Code)
			}
			if code, out := ownerCallMatch(owner, "POST", path, key, "", body); code != 403 {
				t.Fatal("finalization or retained receipt bypassed withdrawn policy", code, out)
			}
			var state string
			var jobs int
			if err := x.db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'artifact_id'=a.id::text) FROM undergrid.artifacts a WHERE id=$1`, upload["id"]).Scan(&state, &jobs); err != nil {
				t.Fatal(err)
			}
			if sealed && (state != "sealed" || jobs != 1) || !sealed && (state != "quarantine" || jobs != 0) {
				t.Fatal("denied command changed normalized effects", state, jobs)
			}
		})
	}
}
