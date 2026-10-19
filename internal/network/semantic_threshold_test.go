package network

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSemanticThresholdBounds(t *testing.T) {
	for _, value := range []float64{-0.01, 1.01, 0, 1, 0.99} {
		t.Run(string(canonical(value)), func(t *testing.T) {
			var body map[string]any
			if err := json.Unmarshal(canonical(intentFixture()), &body); err != nil {
				t.Fatal(err)
			}
			body["semantic_minimum_similarity"] = value
			var input IntentInput
			if err := json.Unmarshal(canonical(body), &input); err != nil {
				t.Fatal(err)
			}
			err := input.validate()
			if (value < 0 || value > 1) && err == nil {
				t.Fatal("out-of-range semantic threshold accepted", value)
			}
			if value >= 0 && value <= 1 && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSemanticThresholdHTTPBoundsAndPrivatePolicy(t *testing.T) {
	s, db, _ := semanticFixture(t)
	owner, outsider := client(t, s), client(t, s)
	var body map[string]any
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Private matching policy " + NewID()
	if err := json.Unmarshal(canonical(input), &body); err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{-0.01, 1.01} {
		body["semantic_minimum_similarity"] = value
		code, result := owner.call("POST", "/v1/intents", "invalid-threshold-"+string(canonical(value)), body)
		if code != 422 {
			t.Fatal(code, result)
		}
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_revisions WHERE document->>'public_summary'=$1", input.PublicSummary).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("invalid threshold persisted", count)
	}
	for _, value := range []float64{0, 1} {
		body["semantic_minimum_similarity"] = value
		code, created := owner.call("POST", "/v1/intents", "boundary-threshold-"+string(canonical(value)), body)
		if code != 201 || created["semantic_minimum_similarity"] != value {
			t.Fatal(code, created)
		}
		id := created["id"].(string)
		code, public := outsider.call("GET", "/v1/intents/"+id, "", nil)
		if code != 200 || public["semantic_minimum_similarity"] != nil {
			t.Fatal("private source policy exposed", code, public)
		}
		var logged float64
		if err := db.QueryRow(context.Background(), "SELECT (facts->>'semantic_minimum_similarity')::double precision FROM undergrid.events WHERE aggregate_id=$1 AND type='intent.created'", id).Scan(&logged); err != nil {
			t.Fatal(err)
		}
		if logged != value {
			t.Fatal("boundary policy not audited", logged, value)
		}
	}
}
