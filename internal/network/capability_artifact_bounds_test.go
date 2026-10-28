package network

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// This opt-in named gate requires a fresh controlled database. Every listed
// artifact is published through normal commands and the actual fixed verifier.
func TestCapabilityFitBoundsNormallyPublishedArtifacts(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	reader := client(t, s)
	input := intentFixture()
	input.Watch.Enabled = false
	code, source := reader.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	sourceID := source["id"].(string)
	t.Cleanup(func() { retireClarificationFixture(t, db, reader, sourceID) })
	root, err := os.MkdirTemp("/tmp", "undergrid-capability-bound-blobs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retained normally published capability blobs: %s", root)
	expected := map[string][2]string{}
	check := func(want int, truncated bool) {
		t.Helper()
		code, report := reader.call("GET", "/v1/intents/"+sourceID+"/capability-fit", "", nil)
		if code != 200 {
			t.Fatal(code, report)
		}
		sample := report["artifacts"].(map[string]any)
		rows := sample["items"].([]any)
		if sample["limit"] != float64(20) || sample["truncated"] != truncated || len(rows) != want {
			t.Fatal("artifact bound differs from real publication count", want, truncated, sample)
		}
		seen := map[string]bool{}
		previous := ""
		for _, raw := range rows {
			row := raw.(map[string]any)
			id := row["artifact_id"].(string)
			hashes, ok := expected[id]
			if !ok || seen[id] || id < previous || row["sha256"] != hashes[0] || row["manifest_hash"] != hashes[1] {
				t.Fatal("sample contains unbound, duplicate or unordered publication", row)
			}
			seen[id] = true
			previous = id
		}
	}
	check(0, false)
	for i := range 21 {
		if !t.Run(fmt.Sprintf("normal_publication_%02d", i+1), func(t *testing.T) {
			x := newArtifactScenario(t)
			x.config.Root = filepath.Join(root, fmt.Sprintf("publication-%02d", i+1))
			if err := os.MkdirAll(x.config.Root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := x.s.SetLocalArtifactRuntime(x.config); err != nil {
				t.Fatal(err)
			}
			data := []byte(`{"sources":[{"source_id":"bounded-publication","source_url":"https://example.org/"}]}`)
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
			if code, out := x.actors[0].call("POST", path+"/publish", NewID(), map[string]any{"manifest_hash": artifact["manifest_hash"]}); code != 200 {
				t.Fatal(code, out)
			}
			expected[id] = [2]string{digest(data), artifact["manifest_hash"].(string)}
		}) {
			t.Fatal("normal publication prerequisite failed")
		}
		if i < 20 {
			check(i+1, false)
		} else {
			check(20, true)
		}
	}
}
