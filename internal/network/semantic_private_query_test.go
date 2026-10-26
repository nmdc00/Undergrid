package network

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPrivateSemanticQueryRequiresPinnedConsentAndStaysOwnerOnly(t *testing.T) {
	s, db, workerDSN := semanticFixture(t)
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	var mu sync.Mutex
	texts := []string{}
	blocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		text := in["input"].(string)
		mu.Lock()
		texts = append(texts, text)
		mu.Unlock()
		if text == "Blocked selected private query" {
			close(blocked)
			<-release
		}
		vector := []float64{1, 0, 0}
		if strings.HasPrefix(text, "Public directory") {
			vector = []float64{0.98, 0.1, 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(text)), "embedding": vector})
	}))
	defer provider.Close()
	defer func() { releaseOnce.Do(func() { close(release) }) }()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	configPath := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "private-fixture", "model": "private-fixture", "model_revision": "r1", "dimensions": 3, "permission_evidence": "controlled fixture only", "privacy_policy_sha256": strings.Repeat("e", 64), "allow_public_projections": true, "allow_private_projections": true, "test_only": true, "tls_root_ca_file": ca}
	if err := os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", configPath)
	a, b := client(t, s), client(t, s)
	cases := []map[string]any{}
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var response, snapshot any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(canonical(body), &snapshot); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": snapshot, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	a.observe, b.observe = observe, observe
	defer func() {
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--private-semantic-query")
		cmd.Stdin = bytes.NewReader(canonical(cases))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("private query actual HTTP contract: %v %s", err, out)
		}
	}()

	code, policy := a.call("GET", "/v1/matching-policy", "", nil)
	if code != 200 || policy["profile_hash"] == nil {
		t.Fatal("reviewable provider policy missing", code, policy)
	}
	for _, secret := range []string{"endpoint", "tls_root_ca_file", "api_key"} {
		if policy[secret] != nil {
			t.Fatal("operator configuration disclosed", policy)
		}
	}
	var body map[string]any
	input := intentFixture()
	input.Objective = "PRIVATE OBJECTIVE NEVER SEND"
	input.Visibility = "private"
	if err := json.Unmarshal(canonical(input), &body); err != nil {
		t.Fatal(err)
	}
	body["private_semantic_query"] = map[string]any{"projection": "Selected environmental citations", "profile_hash": policy["profile_hash"], "acknowledge_external_processing": true}

	for _, scenario := range []struct {
		name  string
		grant map[string]any
		want  int
	}{
		{"missing-ack", map[string]any{"projection": "Selected query", "profile_hash": policy["profile_hash"]}, 422},
		{"declined-ack", map[string]any{"projection": "Selected query", "profile_hash": policy["profile_hash"], "acknowledge_external_processing": false}, 422},
		{"stale-profile", map[string]any{"projection": "Selected query", "profile_hash": strings.Repeat("0", 64), "acknowledge_external_processing": true}, 409},
	} {
		invalid := map[string]any{}
		if err := json.Unmarshal(canonical(body), &invalid); err != nil {
			t.Fatal(err)
		}
		invalid["private_semantic_query"] = scenario.grant
		code, result := a.call("POST", "/v1/intents", scenario.name, invalid)
		if code != scenario.want {
			t.Fatal(scenario.name, code, result)
		}
	}
	code, ownerPolicy := a.call("GET", "/v1/owner/matching-policy", "", nil)
	if code != 200 || ownerPolicy["profile_hash"] != policy["profile_hash"] {
		t.Fatal(code, ownerPolicy)
	}
	code, private := a.call("POST", "/v1/intents", "private-query", body)
	if code != 201 {
		t.Fatal(code, private)
	}
	var recorded string
	if err := db.QueryRow(context.Background(), "SELECT facts->>'private_semantic_profile_hash' FROM undergrid.events WHERE aggregate_id=$1 AND type='intent.created'", private["id"]).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != policy["profile_hash"] {
		t.Fatal("private processing grant not audited", recorded)
	}
	writer, err := pgx.Connect(context.Background(), workerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close(context.Background())
	if _, err = writer.Exec(context.Background(), "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	tx, err := writer.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `UPDATE intent_revisions SET embedding='[1,0,0]'::public.vector,
 embedding_profile_hash=repeat('0',64),embedding_input_hash=encode(sha256(convert_to($2,'UTF8')),'hex'),
 embedding_projection_hash=encode(sha256(convert_to($2,'UTF8')),'hex'),embedding_provider='private-fixture',
 embedding_model='private-fixture',embedding_model_revision='r1',embedding_dimensions=3,embedded_at=clock_timestamp() WHERE intent_id=$1 AND revision=1`, private["id"], "Selected environmental citations")
	_ = tx.Rollback(context.Background())
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
		t.Fatal("database accepted observation with a different consent profile", err)
	}
	input.Visibility = "redacted_public"
	input.PublicSummary = "Public directory of climate references"
	input.Objective = "OTHER PRIVATE OBJECTIVE NEVER SEND"
	code, public := b.call("POST", "/v1/intents", "public-candidate", input)
	if code != 201 {
		t.Fatal(code, public)
	}
	runner := New(workerDSN, "development", nil)
	runner.semantic, err = semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{public["id"].(string), private["id"].(string)} {
		if err := embedThenScanSemanticFixture(context.Background(), runner, id); err != nil {
			t.Fatal(err)
		}
	}
	code, result := a.call("GET", "/v1/intents/"+private["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, result)
	}
	found := false
	for _, value := range result["items"].([]any) {
		if value.(map[string]any)["candidate_intent_id"] == public["id"] {
			found = true
		}
	}
	if !found {
		t.Fatal("selected private projection did not discover public material", result)
	}
	if err := runner.enqueueCompatibleWatchers(context.Background(), watchRefreshArgs{CandidateID: public["id"].(string)}); err != nil {
		t.Fatal(err)
	}
	var refreshed int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$1 AND args->>'watcher_id'=$2`, public["id"], private["id"]).Scan(&refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed == 0 {
		t.Fatal("consented private watcher omitted from committed candidate refresh")
	}
	mu.Lock()
	observed := append([]string(nil), texts...)
	mu.Unlock()
	if len(observed) != 2 {
		t.Fatal("unexpected provider inputs", observed)
	}
	for _, text := range observed {
		if strings.Contains(text, "PRIVATE OBJECTIVE") {
			t.Fatal("private objective leaked", observed)
		}
	}
	code, result = b.call("GET", "/v1/intents/"+private["id"].(string), "", nil)
	if code != 404 {
		t.Fatal("private query became discoverable", code, result)
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.public_intents() WHERE id=$1", private["id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("private source entered candidate projection")
	}
	// Removing consent appends a revision and immediately hides prior matches.
	delete(body, "private_semantic_query")
	code, changed := a.versioned("PATCH", "/v1/intents/"+private["id"].(string), "remove-query-consent", private["etag"].(string), map[string]any{"revision": body})
	if code != 200 {
		t.Fatal(code, changed)
	}
	if err := embedThenScanSemanticFixture(context.Background(), runner, private["id"].(string)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	after := len(texts)
	mu.Unlock()
	if after != 2 {
		t.Fatal("unconsented private revision sent to provider", after)
	}
	if err := runner.enqueueCompatibleWatchers(context.Background(), watchRefreshArgs{CandidateID: public["id"].(string)}); err != nil {
		t.Fatal(err)
	}
	var afterRevocation int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$1 AND args->>'watcher_id'=$2`, public["id"], private["id"]).Scan(&afterRevocation); err != nil {
		t.Fatal(err)
	}
	if afterRevocation != refreshed {
		t.Fatal("revoked private consent admitted a new semantic refresh", refreshed, afterRevocation)
	}

	code, result = a.call("GET", "/v1/intents/"+private["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(result["items"].([]any)) != 0 {
		t.Fatal("removed consent retained stale match", code, result)
	}

	// Removing the exact private grant must finish while provider I/O is blocked.
	body["private_semantic_query"] = map[string]any{"projection": "Blocked selected private query", "profile_hash": policy["profile_hash"], "acknowledge_external_processing": true}
	code, blockedIntent := a.call("POST", "/v1/intents", "private-revoke-in-flight", body)
	if code != 201 {
		t.Fatal(code, blockedIntent)
	}
	scanDone := make(chan error, 1)
	go func() {
		scanDone <- embedThenScanSemanticFixture(context.Background(), runner, blockedIntent["id"].(string))
	}()
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		releaseOnce.Do(func() { close(release) })
		select {
		case <-scanDone:
		case <-time.After(5 * time.Second):
		}
		t.Fatal("private provider call did not enter controlled wait")
	}
	delete(body, "private_semantic_query")
	type mutationResult struct {
		code   int
		result map[string]any
	}
	mutationDone := make(chan mutationResult, 1)
	go func() {
		code, result := a.versioned("PATCH", "/v1/intents/"+blockedIntent["id"].(string), "private-revoke-during-provider", blockedIntent["etag"].(string), map[string]any{"revision": body})
		mutationDone <- mutationResult{code, result}
	}()
	select {
	case mutation := <-mutationDone:
		if mutation.code != 200 {
			releaseOnce.Do(func() { close(release) })
			<-scanDone
			t.Fatal(mutation.code, mutation.result)
		}
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
		select {
		case <-mutationDone:
		case <-time.After(5 * time.Second):
		}
		select {
		case <-scanDone:
		case <-time.After(5 * time.Second):
		}
		t.Fatal("private consent removal held behind provider I/O")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-scanDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("private scan did not finish")
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1 AND embedding IS NOT NULL", blockedIntent["id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("revoked private observation persisted", count)
	}
	// A newly configured worker cannot reuse a previous profile acknowledgment.
	body["private_semantic_query"] = map[string]any{"projection": "Selected environmental citations", "profile_hash": policy["profile_hash"], "acknowledge_external_processing": true}
	code, pending := a.call("POST", "/v1/intents", "pending-profile-cutover", body)
	if code != 201 {
		t.Fatal(code, pending)
	}
	config["model_revision"] = "r2"
	if err := os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	// A still-running worker holding the previous profile must also fail closed.
	if err := embedThenScanSemanticFixture(context.Background(), runner, pending["id"].(string)); err != nil {
		t.Fatal(err)
	}
	runner.semantic, err = semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	if err := embedThenScanSemanticFixture(context.Background(), runner, pending["id"].(string)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	after = len(texts)
	mu.Unlock()
	if after != 3 {
		t.Fatal("new profile reused old private consent", after)
	}
	code, result = a.call("POST", "/v1/intents", "stale-cutover-consent", body)
	if code != 409 {
		t.Fatal("stale profile accepted after cutover", code, result)
	}
	config["allow_private_projections"] = false
	if err := os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	code, result = a.call("POST", "/v1/intents", "disabled-private-consent", body)
	if code != 422 {
		t.Fatal("disabled private processing accepted", code, result)
	}
}
