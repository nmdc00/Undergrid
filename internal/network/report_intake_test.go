package network

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func reportAgent(t *testing.T, s *Service, owner string, scopes []string) testClient {
	t.Helper()
	c := testClient{t: t, s: s, owner: owner}
	if code, out := c.call("POST", "/v1/principals", "report-register", map[string]any{"name": "reporter", "terms_version": "test-v1"}); code != 201 {
		t.Fatalf("register reporter: %d %v", code, out)
	}
	code, out := c.call("POST", "/v1/agents", "report-agent", map[string]any{"name": "report agent", "scopes": scopes})
	if code != 201 {
		t.Fatalf("create report agent: %d %v", code, out)
	}
	c.token = out["token"].(string)
	return c
}

func restrictedReportDSN(t *testing.T, db *pgx.Conn, role string) string {
	t.Helper()
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_report_api_runner') THEN
  CREATE ROLE undergrid_report_api_runner LOGIN NOSUPERUSER NOBYPASSRLS;
 END IF;
 END $$; GRANT undergrid_app TO undergrid_report_api_runner`); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	q := url.Values{"host": {cfg.Host}, "port": {strconv.Itoa(int(cfg.Port))}, "sslmode": {"disable"}}
	u := url.URL{Scheme: "postgres", User: url.User(role), Path: "/" + cfg.Database, RawQuery: q.Encode()}
	return u.String()
}

func TestAuthenticatedReportReceiptIsBoundAndPrivate(t *testing.T) {
	s, db := fixture(t)
	if err := s.ConfigureReportTriageWindow(2 * time.Hour); err != nil {
		t.Fatal("configure internal triage target", err)
	}
	runtimeDSN := restrictedReportDSN(t, db, "undergrid_report_api_runner")
	runtimeDB, err := pgx.Connect(context.Background(), runtimeDSN)
	if err != nil {
		t.Fatal("connect restricted API login", err)
	}
	var sessionUser string
	var superuser, bypassRLS bool
	if err = runtimeDB.QueryRow(context.Background(), `SELECT session_user,rolsuper,rolbypassrls FROM pg_roles WHERE rolname=session_user`).Scan(&sessionUser, &superuser, &bypassRLS); err != nil {
		runtimeDB.Close(context.Background())
		t.Fatal(err)
	}
	if err = runtimeDB.Close(context.Background()); err != nil || sessionUser != "undergrid_report_api_runner" || superuser || bypassRLS {
		t.Fatalf("unsafe report API role %q super=%v bypass=%v err=%v", sessionUser, superuser, bypassRLS, err)
	}
	s.dsn = runtimeDSN
	ctx := context.Background()
	reader := reportAgent(t, s, "report-reader-"+NewID(), []string{"reports:read"})
	writer := reportAgent(t, s, "report-writer-"+NewID(), []string{"reports:write", "reports:read"})
	outsider := reportAgent(t, s, "unrelated-owner-"+NewID(), []string{"reports:read"})

	var writerPrincipal, writerAgent string
	if code, me := writer.call("GET", "/v1/me", "", nil); code != 200 {
		t.Fatal("read reporter identity", code, me)
	} else {
		writerPrincipal, writerAgent = me["principal_id"].(string), me["agent_id"].(string)
	}
	_, outsiderIdentity := outsider.call("GET", "/v1/me", "", nil)
	now := time.Now()
	subjectID, evidenceID := NewID(), NewID()
	body := map[string]any{
		"kind":         "privacy",
		"subject":      map[string]any{"kind": "artifact", "id": subjectID},
		"evidence_ref": map[string]any{"kind": "message", "id": evidenceID},
		"statement":    "A private artifact was exposed to an unrelated reader.",
	}
	if code, out := reader.call("POST", "/v1/reports", "without-write-scope", body); code != 403 {
		t.Fatalf("read-only credential submitted report: %d %v", code, out)
	}
	code, receipt := writer.call("POST", "/v1/reports", "submit-case", body)
	if code != 201 {
		t.Fatalf("submit report: %d %v", code, receipt)
	}
	id, ok := receipt["id"].(string)
	if !ok || receipt["kind"] != "privacy" || receipt["state"] != "pending" {
		t.Fatalf("invalid private report receipt: %v", receipt)
	}
	if _, exists := receipt["statement"]; exists {
		t.Fatal("submission receipt returned the confidential statement")
	}
	var reporterPrincipal, reporterAgent, storedKind, storedSubjectKind, storedSubjectID, storedEvidenceKind, storedEvidenceID, statement, state string
	var triageDue time.Time
	if err := db.QueryRow(ctx, `SELECT reporter_principal_id::text,reporter_agent_id::text,kind,subject_kind,subject_id::text,
 evidence_kind,evidence_id::text,statement,state,triage_due_at FROM undergrid.reports WHERE id=$1`, id).Scan(
		&reporterPrincipal, &reporterAgent, &storedKind, &storedSubjectKind, &storedSubjectID, &storedEvidenceKind, &storedEvidenceID, &statement, &state, &triageDue); err != nil {
		t.Fatal("load normalized report", err)
	}
	if reporterPrincipal != writerPrincipal || reporterAgent != writerAgent || storedKind != "privacy" || storedSubjectKind != "artifact" || storedSubjectID != subjectID || storedEvidenceKind != "message" || storedEvidenceID != evidenceID || statement != body["statement"] || state != "pending" {
		t.Fatalf("stored report was not bound to credential/scope: reporter=%s/%s kind=%s subject=%s/%s evidence=%s/%s state=%s", reporterPrincipal, reporterAgent, storedKind, storedSubjectKind, storedSubjectID, storedEvidenceKind, storedEvidenceID, state)
	}
	if triageDue.Before(now.Add(105*time.Minute)) || triageDue.After(now.Add(135*time.Minute)) {
		t.Fatalf("internal triage deadline outside configured window: now=%s due=%s", now, triageDue)
	}
	var eventFacts []byte
	if err := db.QueryRow(ctx, `SELECT facts FROM undergrid.events WHERE aggregate_id=$1 AND type='report.submitted'`, id).Scan(&eventFacts); err != nil {
		t.Fatal("safe submission event missing", err)
	}
	for _, private := range []string{body["statement"].(string), subjectID, evidenceID, writerPrincipal, writerAgent} {
		if strings.Contains(string(eventFacts), private) {
			t.Fatalf("event facts disclose private report input %q: %s", private, eventFacts)
		}
	}
	var events int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("unexpected report events: %d %v", events, err)
	}

	// Unknown typed subjects are accepted without lookup or an existence oracle.
	unknownID := NewID()
	unknown := map[string]any{"kind": "bug", "subject": map[string]any{"kind": "task", "id": unknownID}, "statement": "The referenced task appears unavailable."}
	if unknownReceiptCode, unknownReceipt := writer.call("POST", "/v1/reports", "unknown-subject", unknown); unknownReceiptCode != 201 || unknownReceipt["state"] != "pending" {
		t.Fatalf("unknown subject was resolved or rejected: %d %v", unknownReceiptCode, unknownReceipt)
	}
	if malformedCode, malformed := writer.call("POST", "/v1/reports", "spoof-reporter", map[string]any{
		"kind": "abuse", "subject": map[string]any{"kind": "agent", "id": NewID()}, "statement": "spoof",
		"reporter_principal_id": outsider.owner, "reporter_agent_id": NewID(),
	}); malformedCode != 400 {
		t.Fatalf("client-supplied reporter identity was accepted: %d %v", malformedCode, malformed)
	}
	if invalidCode, invalid := writer.call("POST", "/v1/reports", "invalid-kind", map[string]any{
		"kind": "payment-dispute", "subject": map[string]any{"kind": "agent", "id": NewID()}, "statement": "unsupported kind",
	}); invalidCode != 422 {
		t.Fatalf("unsupported report type was accepted: %d %v", invalidCode, invalid)
	}
	if oversizedCode, oversized := writer.call("POST", "/v1/reports", "oversized", map[string]any{
		"kind": "bug", "subject": map[string]any{"kind": "agent", "id": NewID()}, "statement": strings.Repeat("x", 4001),
	}); oversizedCode != 422 {
		t.Fatalf("oversized statement was accepted: %d %v", oversizedCode, oversized)
	}
	if attachedCode, attached := writer.call("POST", "/v1/reports", "attachment", map[string]any{
		"kind": "rights", "subject": map[string]any{"kind": "artifact", "id": NewID()}, "statement": "attachment test", "attachment": "untrusted bytes",
	}); attachedCode != 400 {
		t.Fatalf("attachment content was accepted: %d %v", attachedCode, attached)
	}

	if replayCode, replay := writer.call("POST", "/v1/reports", "submit-case", body); replayCode != 201 || replay["id"] != id {
		t.Fatalf("same-key retry created a new report: %d %v", replayCode, replay)
	}
	changed := map[string]any{"kind": "privacy", "subject": body["subject"], "evidence_ref": body["evidence_ref"], "statement": "changed after acknowledgement"}
	if conflictCode, conflict := writer.call("POST", "/v1/reports", "submit-case", changed); conflictCode != 409 {
		t.Fatalf("changed same-key report did not conflict: %d %v", conflictCode, conflict)
	}
	if deniedCode, denied := outsider.call("GET", "/v1/reports/"+id, "", nil); deniedCode != 404 {
		t.Fatalf("unrelated principal discovered private report: %d %v", deniedCode, denied)
	}
	if deniedCode, denied := reader.call("GET", "/v1/reports/"+id, "", nil); deniedCode != 404 {
		t.Fatalf("different reporter found another principal's report: %d %v", deniedCode, denied)
	}

	// The HTTP login is a real NOLOGIN/NOSUPERUSER/NOBYPASSRLS member. Even
	// with a different principal context, direct table reads cannot cross RLS.
	foreign, err := pgx.Connect(ctx, runtimeDSN)
	if err != nil {
		t.Fatal("connect restricted report login", err)
	}
	defer foreign.Close(ctx)
	tx, err := foreign.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", outsiderIdentity["principal_id"], outsiderIdentity["agent_id"]); err != nil {
		t.Fatal(err)
	}
	var foreignRows int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM reports WHERE id=$1", id).Scan(&foreignRows); err != nil || foreignRows != 0 {
		t.Fatalf("restricted SQL role read foreign report: rows=%d err=%v", foreignRows, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// A fresh handler recovers the same reporter's acknowledged private case.
	writer.s = New(runtimeDSN, "development", nil)
	if getCode, report := writer.call("GET", "/v1/reports/"+id, "", nil); getCode != 200 {
		t.Fatalf("report did not survive handler restart: %d %v", getCode, report)
	} else if report["statement"] != body["statement"] || report["subject"].(map[string]any)["id"] != subjectID {
		t.Fatalf("restart changed reporter-visible case: %v", report)
	}
}

func TestReportIntakeBoundsAndScopeSeparation(t *testing.T) {
	s, _ := fixture(t)
	if err := s.ConfigureReportTriageWindow(31 * 24 * time.Hour); err == nil {
		t.Fatal("unbounded internal triage target was accepted")
	}
	writer := reportAgent(t, s, "report-bounds-"+NewID(), []string{"reports:write"})
	valid := map[string]any{"kind": "abuse", "subject": map[string]any{"kind": "message", "id": NewID()}, "statement": "A bounded report."}
	if code, out := writer.call("GET", "/v1/reports/"+NewID(), "", nil); code != 403 {
		t.Fatalf("write-only credential read report: %d %v", code, out)
	}
	if code, out := writer.call("POST", "/v1/reports", "blank", map[string]any{"kind": "bug", "subject": valid["subject"], "statement": " \n\t "}); code != 422 {
		t.Fatalf("blank report was accepted: %d %v", code, out)
	}
	if code, out := writer.call("POST", "/v1/reports", "bad-reference", map[string]any{"kind": "bug", "subject": map[string]any{"kind": "unknown", "id": NewID()}, "statement": "bad reference"}); code != 422 {
		t.Fatalf("untyped subject was accepted: %d %v", code, out)
	}
	if code, out := writer.call("POST", "/v1/reports", "url-reference", map[string]any{"kind": "privacy", "subject": "https://internal.example/private", "statement": "must not fetch"}); code != 400 {
		t.Fatalf("remote/untyped reference was accepted: %d %v", code, out)
	}
}

func TestReportIntakeRateCeilingIsPrincipalScoped(t *testing.T) {
	s, db := fixture(t)
	owner := "report-rate-" + NewID()
	writer := reportAgent(t, s, owner, []string{"reports:write"})
	code, issued := writer.call("POST", "/v1/agents", "report-sibling-agent", map[string]any{"name": "report sibling", "scopes": []string{"reports:write"}})
	if code != 201 {
		t.Fatalf("issue sibling report agent: %d %v", code, issued)
	}
	sibling := writer
	sibling.token = issued["token"].(string)
	body := map[string]any{"kind": "bug", "subject": map[string]any{"kind": "agent", "id": NewID()}, "statement": "A repeated test report."}
	for i := 0; i < 29; i++ {
		actor := writer
		if i%2 == 1 {
			actor = sibling
		}
		if code, out := actor.call("POST", "/v1/reports", "rate-"+strconv.Itoa(i), body); code != 201 {
			t.Fatalf("report %d before ceiling: %d %v", i, code, out)
		}
	}
	start := make(chan struct{})
	type submission struct {
		code int
		out  map[string]any
	}
	results := make(chan submission, 2)
	var requests sync.WaitGroup
	requests.Add(2)
	for i, actor := range []testClient{writer, sibling} {
		go func(i int, actor testClient) {
			defer requests.Done()
			<-start
			code, out := actor.call("POST", "/v1/reports", "rate-race-"+strconv.Itoa(i), body)
			results <- submission{code, out}
		}(i, actor)
	}
	close(start)
	requests.Wait()
	close(results)
	accepted, denied := 0, 0
	for result := range results {
		switch result.code {
		case 201:
			accepted++
		case 429:
			if result.out["error"].(map[string]any)["code"] != "report_write_rate_exceeded" {
				t.Fatalf("concurrent rate denial had wrong error: %v", result.out)
			}
			denied++
		default:
			t.Fatalf("concurrent submission returned unexpected status: %d %v", result.code, result.out)
		}
	}
	if accepted != 1 || denied != 1 {
		t.Fatalf("concurrent final-slot admissions: accepted=%d denied=%d", accepted, denied)
	}
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.reports
 WHERE reporter_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)`, owner).Scan(&count); err != nil || count != 30 {
		t.Fatalf("rate denial persisted a report: count=%d err=%v", count, err)
	}
	var reportEvents int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.events
 WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND type='report.submitted'`, owner).Scan(&reportEvents); err != nil || reportEvents != 30 {
		t.Fatalf("rate denial wrote an event or accepted report lacks one: report events=%d err=%v", reportEvents, err)
	}
}
