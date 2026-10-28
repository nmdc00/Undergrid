package network

import "testing"

func TestCapabilityFitDiscoversOnlyNormallyPublishedArtifactMetadata(t *testing.T) {
	x := newArtifactScenario(t)
	reader := x.actors[3]
	input := intentFixture()
	input.Watch.Enabled = false
	code, source := reader.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	sourceID := source["id"].(string)
	t.Cleanup(func() { retireClarificationFixture(t, x.db, reader, sourceID) })
	data := []byte(`{"sources":[{"source_id":"capability-test","source_url":"https://example.org/"}]}`)
	upload := x.upload(t, data, NewID())
	id := upload["id"].(string)
	path := "/v1/artifacts/" + id
	code, artifact := x.actors[2].call("POST", path+"/finalize", NewID(), x.finalBody(data))
	if code != 200 {
		t.Fatal(code, artifact)
	}
	x.startWorker(t)
	evaluation := x.waitEvaluation(t, id)
	if evaluation["result"] != "passed" {
		t.Fatal(evaluation)
	}
	for _, actor := range x.actors[:2] {
		if code, out := actor.call("POST", path+"/acceptances", NewID(), map[string]any{"evaluation_id": evaluation["id"], "artifact_sha256": digest(data), "plan_hash": x.plan["content_hash"]}); code != 200 {
			t.Fatal(code, out)
		}
	}
	for _, actor := range x.actors[:3] {
		if code, out := actor.call("POST", path+"/publication-approvals", NewID(), map[string]any{"manifest_hash": artifact["manifest_hash"], "license_id": "CC0-1.0"}); code != 200 {
			t.Fatal(code, out)
		}
	}
	read := func() []any {
		t.Helper()
		code, report := reader.call("GET", "/v1/intents/"+sourceID+"/capability-fit", "", nil)
		if code != 200 {
			t.Fatal(code, report)
		}
		sample, ok := report["artifacts"].(map[string]any)
		if !ok {
			t.Fatal("published-artifact capability sample absent", report)
		}
		items, ok := sample["items"].([]any)
		if !ok {
			t.Fatal("missing bounded artifact sample", sample)
		}
		return items
	}
	// Verification and acceptance alone are not publication.
	for _, raw := range read() {
		if raw.(map[string]any)["artifact_id"] == id {
			t.Fatal("unpublished artifact exposed", raw)
		}
	}
	if code, out := x.actors[0].call("POST", path+"/publish", NewID(), map[string]any{"manifest_hash": artifact["manifest_hash"]}); code != 200 {
		t.Fatal(code, out)
	}
	found := false
	for _, raw := range read() {
		row := raw.(map[string]any)
		if row["artifact_id"] != id {
			continue
		}
		found = true
		if row["sha256"] != digest(data) || row["manifest_hash"] != artifact["manifest_hash"] || row["origin"] != "registered_capability" || row["schema_family"] != "source_catalog_v1" || row["license_id"] != "CC0-1.0" {
			t.Fatal("unbound public capability", row)
		}
		for _, key := range []string{"swarm_id", "task_id", "plan_id", "inputs", "attribution", "required_rights", "content"} {
			if _, ok := row[key]; ok {
				t.Fatal("private context in capability", key, row)
			}
		}
	}
	if !found {
		t.Fatal("normally published capability absent", id)
	}
}
