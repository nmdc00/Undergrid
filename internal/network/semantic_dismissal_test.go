package network

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSemanticSuggestionDismissalRequiresCurrentEvidenceBeforeReplay(t *testing.T) {
	provider, _ := controlledSemanticConsent(t)
	s, db, worker, _ := externalNotificationFixture(t)
	worker.semantic = provider
	a, b := client(t, s), client(t, s)
	ids := []string{}
	created := []map[string]any{}
	for n, owner := range []testClient{a, b} {
		input := intentFixture()
		input.Visibility = "redacted_public"
		input.PublicSummary = []string{"Environmental regulation reference catalog", "Climate agency citation directory"}[n]
		code, out := owner.call("POST", "/v1/intents", NewID(), input)
		if code != 201 {
			t.Fatal(code, out)
		}
		ids = append(ids, out["id"].(string))
		created = append(created, out)
	}
	connection, err := pgx.Connect(context.Background(), worker.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	if _, err = connection.Exec(context.Background(), "SET ROLE undergrid_worker;SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err = worker.ensureSemanticEmbedding(context.Background(), connection, id); err != nil {
			t.Fatal(err)
		}
	}
	if err = worker.scanIntentCandidate(context.Background(), connection, ids[0], ids[1], true); err != nil {
		t.Fatal(err)
	}
	code, page := a.call("GET", "/v1/intents/"+ids[0]+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	if len(page["items"].([]any)) != 1 {
		t.Fatal("semantic precondition missing", page)
	}
	item := page["items"].([]any)[0].(map[string]any)
	evidence := item["evidence"].(map[string]any)
	if evidence["algorithm"] != "hard-constraints-hybrid-cosine-v1" {
		t.Fatal("not semantic", evidence)
	}
	path := "/v1/opportunities/" + item["id"].(string) + "/dismiss"
	for range 2 {
		code, out := a.call("POST", path, "semantic-dismiss", map[string]any{})
		if code != 200 || out["state"] != "dismissed" {
			t.Fatalf("current semantic dismissal/retry denied: %d %v", code, out)
		}
	}
	if err = worker.scanIntentCandidate(context.Background(), connection, ids[0], ids[1], true); err != nil {
		t.Fatal(err)
	}
	var suggestions, events int
	if err = db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$3 AND type='opportunity.dismissed')`, ids[0], ids[1], item["id"]).Scan(&suggestions, &events); err != nil {
		t.Fatal(err)
	}
	if suggestions != 1 || events != 1 {
		t.Fatal("duplicate unchanged suggestion/dismissal", suggestions, events)
	}
	code, out := b.versioned("PATCH", "/v1/intents/"+ids[1], "withdraw-semantic-candidate", created[1]["etag"].(string), map[string]any{"state": "withdrawn"})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = a.call("POST", path, "semantic-dismiss", map[string]any{})
	if code != 404 {
		t.Fatal("obsolete semantic receipt recovered after withdrawal", code, out)
	}
}
