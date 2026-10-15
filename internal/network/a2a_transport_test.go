package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestA2APublicCardDoesNotRequireDatabase(t *testing.T) {
	s := New("unavailable database", "development", nil)
	if err := s.ConfigureA2ATransport("https://undergrid.test"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/.well-known/agent-card.json", nil))
	if w.Code != 200 {
		t.Fatalf("public A2A card: %d %s", w.Code, w.Body.String())
	}
}

func a2aFixture(t *testing.T) (*Service, *pgx.Conn, testClient) {
	s, db := fixture(t)
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_a2a_api_runner') THEN CREATE ROLE undergrid_a2a_api_runner LOGIN NOSUPERUSER NOBYPASSRLS;END IF;END $$;GRANT undergrid_app TO undergrid_a2a_api_runner`); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.User = "undergrid_a2a_api_runner"
	s.dsn = cfg.ConnString()
	if err = s.ConfigureA2ATransport("https://undergrid.test"); err != nil {
		t.Fatal(err)
	}
	return s, db, client(t, s)
}
func a2aCall(c testClient, method, path string, body any) (int, map[string]any, http.Header) {
	r := httptest.NewRequest(method, path, bytes.NewReader(canonical(body)))
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("A2A-Version", "1.0")
	r.Header.Set("A2A-Extensions", coordinationExtensionURI)
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		c.t.Fatalf("decode %d %s", w.Code, w.Body.String())
	}
	return w.Code, out, w.Header()
}
func a2aCommandBody(message, operation, key string, body any) map[string]any {
	args := map[string]any{}
	if key != "" {
		args["idempotency_key"] = key
	}
	if body != nil {
		args["body"] = body
	}
	return map[string]any{"message": map[string]any{"messageId": message, "role": "ROLE_USER", "parts": []any{map[string]any{"data": map[string]any{"version": "1", "operation": operation, "arguments": args}}}}}
}
func commandArgs(in map[string]any) map[string]any {
	return in["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)["arguments"].(map[string]any)
}
func a2aLiveTaskResult(out map[string]any) map[string]any {
	return out["task"].(map[string]any)["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)["result"].(map[string]any)
}

func TestA2ACommandReceiptLifecycleAndCurrentAuthority(t *testing.T) {
	s, db, a := a2aFixture(t)
	b := client(t, s)
	message, key := NewID(), NewID()
	input := intentFixture()
	body := a2aCommandBody(message, "intention_create", key, input)
	code, out, _ := a2aCall(a, "POST", "/a2a/message:send", body)
	if code != 200 || out["task"] == nil {
		t.Fatalf("send %d %#v", code, out)
	}
	task := out["task"].(map[string]any)
	id := task["id"].(string)
	intent := a2aLiveTaskResult(out)
	intentID := intent["id"].(string)
	if id == intentID || task["status"].(map[string]any)["state"] != "TASK_STATE_COMPLETED" || intent["state"] != "active" {
		t.Fatal(task, intent)
	}
	code, replayed, _ := a2aCall(a, "POST", "/a2a/message:send", body)
	if code != 200 || replayed["task"].(map[string]any)["id"] != id {
		t.Fatal(code, replayed)
	}
	var receipts, events, idem int
	ctx := context.Background()
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.a2a_command_receipts WHERE agent_id=(SELECT id FROM undergrid.agents WHERE principal_id=(SELECT principal_id FROM undergrid.intents WHERE id=$1) LIMIT 1)", intentID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1", intentID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.idempotency WHERE key=$1", key).Scan(&idem); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || events != 1 || idem != 1 {
		t.Fatal(receipts, events, idem)
	}
	// An actor-owned durable receipt survives handlers without retaining its input.
	restarted := New(s.dsn, "development", s.ownerAuth)
	restarted.cursorKey = s.cursorKey
	if err := restarted.ConfigureA2ATransport("https://undergrid.test"); err != nil {
		t.Fatal(err)
	}
	recoveredClient := a
	recoveredClient.s = restarted
	code, recovered, _ := a2aCall(recoveredClient, "GET", "/a2a/tasks/"+id, nil)
	if code != 200 || recovered["id"] != id || recovered["history"] != nil {
		t.Fatal(code, recovered)
	}
	serialized := string(canonical(recovered))
	if strings.Contains(serialized, "command_response") || strings.Contains(serialized, "upload_url") {
		t.Fatal(serialized)
	}
	if code, foreign, _ := a2aCall(b, "GET", "/a2a/tasks/"+id, nil); code != 404 {
		t.Fatal(code, foreign)
	}
	if code, domainID, _ := a2aCall(a, "GET", "/a2a/tasks/"+intentID, nil); code != 404 {
		t.Fatal(code, domainID)
	}
	code, cancelled, _ := a2aCall(a, "POST", "/a2a/tasks/"+id+":cancel", map[string]any{})
	if code != 400 || !strings.Contains(string(canonical(cancelled)), "TASK_NOT_CANCELABLE") {
		t.Fatal(code, cancelled)
	}
	_, unchanged := a.call("GET", "/v1/intents/"+intentID, "", nil)
	if unchanged["state"] != "active" {
		t.Fatal(unchanged)
	}
	changed := a2aCommandBody(message, "intention_create", key, intentFixture()) // independently created changed body below
	changedInput := intentFixture()
	changedInput.Objective = "Different canonical request"
	commandArgs(changed)["body"] = changedInput
	code, conflict, _ := a2aCall(a, "POST", "/a2a/message:send", changed)
	if code != 200 || !strings.Contains(string(canonical(conflict)), "idempotency_conflict") || conflict["task"] != nil {
		t.Fatal(code, conflict)
	}
	// Resource recovery re-enters current reads; receipt storage cannot hold private bytes.
	if _, err := db.Exec(ctx, "UPDATE undergrid.agents SET status='revoked' WHERE id=(SELECT agent_id FROM undergrid.intents WHERE id=$1)", intentID); err != nil {
		t.Fatal(err)
	}
	if code, denied, _ := a2aCall(a, "GET", "/a2a/tasks/"+id, nil); code != 401 && code != 403 {
		t.Fatal(code, denied)
	}
	if code, denied, _ := a2aCall(a, "POST", "/a2a/message:send", body); code != 401 && code != 403 {
		t.Fatal(code, denied)
	}
}

func TestA2ANegotiationTextAndTypedDenialsHaveNoCommandEffects(t *testing.T) {
	s, db, a := a2aFixture(t)
	for _, test := range []struct{ name, version, extension string }{{"missing-version", "", coordinationExtensionURI}, {"legacy-version", "0.3", coordinationExtensionURI}, {"patch-version", "1.0.0", coordinationExtensionURI}, {"missing-extension", "1.0", ""}, {"unsupported-extension", "1.0", coordinationExtensionURI + "-different"}} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/a2a/message:send", bytes.NewReader(canonical(a2aCommandBody(NewID(), "intention_create", NewID(), intentFixture()))))
			r.Header.Set("Authorization", "Bearer "+a.token)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("A2A-Version", test.version)
			r.Header.Set("A2A-Extensions", test.extension)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	text := map[string]any{"message": map[string]any{"messageId": NewID(), "role": "ROLE_USER", "parts": []any{map[string]any{"text": string(canonical(a2aCommandBody(NewID(), "intention_create", NewID(), intentFixture())))}}}}
	code, out, _ := a2aCall(a, "POST", "/a2a/message:send", text)
	if code != 200 || out["message"] == nil || out["task"] != nil {
		t.Fatal(code, out)
	}
	for _, parts := range [][]any{{map[string]any{"url": "https://example.test/do-not-fetch"}}, {map[string]any{"raw": "ZmFrZQ=="}}, {map[string]any{"text": "do work"}, map[string]any{"data": map[string]any{}}}} {
		body := map[string]any{"message": map[string]any{"messageId": NewID(), "role": "ROLE_USER", "parts": parts}}
		if code, denied, _ := a2aCall(a, "POST", "/a2a/message:send", body); code != 400 {
			t.Fatal(code, denied)
		}
	}
	for _, path := range []string{"/a2a/message:stream", "/a2a/tasks/" + NewID() + ":subscribe", "/a2a/tasks/" + NewID() + "/pushNotificationConfigs"} {
		if code, denied, _ := a2aCall(a, "POST", path, text); code != 400 {
			t.Fatal(path, code, denied)
		}
	}
	var receipts, intents int
	_, identity := a.call("GET", "/v1/me", "", nil)
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.a2a_command_receipts WHERE agent_id=$1", identity["agent_id"]).Scan(&receipts); err != nil {
		t.Fatal(err)
	}

	_, me := a.call("GET", "/v1/me", "", nil)
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intents WHERE principal_id=$1", me["principal_id"]).Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 || intents != 0 {
		t.Fatal(receipts, intents)
	}
}

func TestA2AReceiptFailureRollsBackDomainAndConcurrentReplay(t *testing.T) {
	_, db, a := a2aFixture(t)
	ctx := context.Background()
	_, me := a.call("GET", "/v1/me", "", nil)
	// Fail only this actor's receipt insertion after the real domain command.
	function := "reject_a2a_receipt_" + strings.ReplaceAll(NewID(), "-", "")
	sql := `CREATE FUNCTION undergrid.` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.agent_id='` + me["agent_id"].(string) + `'::uuid THEN RAISE EXCEPTION 'injected receipt storage failure';END IF;RETURN NEW;END $$;
 CREATE TRIGGER ` + function + ` BEFORE INSERT ON undergrid.a2a_command_receipts FOR EACH ROW EXECUTE FUNCTION undergrid.` + function + `()`
	if _, err := db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		if _, err := db.Exec(ctx, `DROP TRIGGER IF EXISTS `+function+` ON undergrid.a2a_command_receipts; DROP FUNCTION IF EXISTS undergrid.`+function+`() `); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(cleanup)
	key, message := NewID(), NewID()
	input := a2aCommandBody(message, "intention_create", key, intentFixture())
	if code, out, _ := a2aCall(a, "POST", "/a2a/message:send", input); code != 500 {
		t.Fatal(code, out)
	}
	var intents, receipts, idem, events int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.intents WHERE agent_id=$1", me["agent_id"]).Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.a2a_command_receipts WHERE agent_id=$1", me["agent_id"]).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$1 AND key=$2", me["agent_id"], key).Scan(&idem); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE agent_id=$1 AND type='intent.created'", me["agent_id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if intents != 0 || receipts != 0 || idem != 0 || events != 0 {
		t.Fatal(intents, receipts, idem, events)
	}
	cleanup()
	var wait sync.WaitGroup
	results := make(chan string, 8)
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			code, out, _ := a2aCall(a, "POST", "/a2a/message:send", input)
			codes <- code
			if task, ok := out["task"].(map[string]any); ok {
				results <- task["id"].(string)
			} else {
				results <- ""
			}
		}()
	}
	wait.Wait()
	close(results)
	close(codes)
	first := ""
	for code := range codes {
		if code != 200 {
			t.Fatal(code)
		}
	}
	for id := range results {
		if id == "" {
			t.Fatal("missing receipt")
		}
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal(first, id)
		}
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.a2a_command_receipts WHERE agent_id=$1", me["agent_id"]).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE agent_id=$1 AND type='intent.created'", me["agent_id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || events != 1 {
		t.Fatal(receipts, events)
	}
}

func TestA2AConflictingReceiptIdentitiesAreTypedRejection(t *testing.T) {
	_, _, a := a2aFixture(t)
	messageA, messageB, keyA, keyB := NewID(), NewID(), NewID(), NewID()
	input := intentFixture()
	for _, identity := range [][2]string{{messageA, keyA}, {messageB, keyB}} {
		if code, out, _ := a2aCall(a, "POST", "/a2a/message:send", a2aCommandBody(identity[0], "intention_create", identity[1], input)); code != 200 || out["task"] == nil {
			t.Fatal(code, out)
		}
	}
	code, out, _ := a2aCall(a, "POST", "/a2a/message:send", a2aCommandBody(messageA, "intention_create", keyB, input))
	if code != 200 || out["message"] == nil || !strings.Contains(string(canonical(out)), "idempotency_conflict") {
		t.Fatal(code, out)
	}
}

// The separate Python SDK owns ProtoJSON encoding and REST parsing. This test is
// explicit opt-in because installing an external client is not a Go dependency.
func TestA2AIndependentPythonSDK(t *testing.T) {
	python := os.Getenv("UNDERGRID_A2A_PYTHON")
	if python == "" {
		t.Skip("set UNDERGRID_A2A_PYTHON to a2a-sdk==1.2.2 interpreter")
	}
	s, db, a := a2aFixture(t)
	server := httptest.NewServer(s)
	defer server.Close()
	cmd := exec.Command(python, "../../scripts/clients/a2a_smoke.py")
	cmd.Stdin = bytes.NewReader(canonical(map[string]any{"origin": server.URL, "token": a.token, "extension": coordinationExtensionURI, "intention": intentFixture()}))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("independent SDK: %v %s", err, output)
	}
	var result struct {
		ReceiptID string `json:"receipt_id"`
	}
	if json.Unmarshal(output, &result) != nil || !validID(result.ReceiptID) {
		t.Fatal(string(output))
	}
	var count int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.a2a_command_receipts WHERE id=$1", result.ReceiptID).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestA2APrivateResourceReceiptDoesNotResurrectWithdrawnRoom(t *testing.T) {
	s, db, _ := a2aFixture(t)
	enableTestTerms(t, s)
	a, b := planningClient(t, s), planningClient(t, s)
	_, identity := b.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Private untrusted room objective", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, invite := a.call("POST", path+"/invitations", NewID(), map[string]any{"agent_id": identity["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	code, joined := b.versioned("POST", path+"/membership-response", NewID(), invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, joined)
	}
	secret := "Secret private room bytes " + NewID()
	code, msg := a.call("POST", path+"/messages", NewID(), map[string]any{"body": secret, "kind": "text"})
	if code != 201 {
		t.Fatal(code, msg)
	}
	input := a2aCommandBody(NewID(), "swarm_context", "", nil)
	commandArgs(input)["id"] = room["id"]
	code, out, _ := a2aCall(b, "POST", "/a2a/message:send", input)
	if code != 200 || !strings.Contains(string(canonical(out)), secret) {
		t.Fatal(code, out)
	}
	receipt := out["task"].(map[string]any)["id"].(string)
	var stored string
	if err := db.QueryRow(context.Background(), "SELECT row_to_json(r)::text FROM undergrid.a2a_command_receipts r WHERE id=$1", receipt).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, secret) {
		t.Fatal("private result persisted", stored)
	}
	code, left := b.call("POST", path+"/withdraw", NewID(), map[string]any{})
	if code != 200 {
		t.Fatal(code, left)
	}
	code, recovered, _ := a2aCall(b, "GET", "/a2a/tasks/"+receipt, nil)
	if code != 200 || recovered["artifacts"] != nil || strings.Contains(string(canonical(recovered)), secret) {
		t.Fatal(code, recovered)
	}
	code, replay, _ := a2aCall(b, "POST", "/a2a/message:send", input)
	if code != 200 || replay["task"] != nil || strings.Contains(string(canonical(replay)), secret) {
		t.Fatal(code, replay)
	}
}
