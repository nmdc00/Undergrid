package network

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// These controlled TLS observations prove current operator authority, not real
// provider/model qualification. Both runtime connections are independently
// verified restricted LOGINs through semanticFixture.
func TestSemanticPublicProfileAuthorityBeforeProviderIO(t *testing.T) {
	for _, change := range []string{"removed", "replaced", "public_disabled"} {
		t.Run(change, func(t *testing.T) { semanticPublicProfileAuthority(t, change, false) })
	}
}
func TestSemanticPublicProfileAuthorityBeforeDurableWrite(t *testing.T) {
	for _, change := range []string{"removed", "replaced", "public_disabled"} {
		t.Run(change, func(t *testing.T) { semanticPublicProfileAuthority(t, change, true) })
	}
}
func semanticPublicProfileAuthority(t *testing.T, change string, inFlight bool) {
	t.Helper()
	s, db, workerDSN := semanticFixture(t)
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Approved public authority projection " + NewID()
	input.Objective = "PRIVATE OBJECTIVE MUST NOT LEAVE"
	var calls atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Model    string `json:"model"`
			Revision string `json:"model_revision"`
			Input    string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Input != input.PublicSummary {
			http.Error(w, "outside approved projection", 422)
			return
		}
		enteredOnce.Do(func() { close(entered) })
		if inFlight {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": request.Model, "model_revision": request.Revision, "input_sha256": digest([]byte(request.Input)), "embedding": []float64{1, 0, 0}})
	}))
	defer provider.Close()
	defer unblock()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	path := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := semanticConfig{Endpoint: provider.URL, ProviderID: "public-authority-fixture", Model: "controlled", ModelRevision: "r1", Dimensions: 3, PermissionEvidence: "controlled operator policy fixture only", PrivacyPolicySHA256: strings.Repeat("e", 64), AllowPublicProjections: true, AllowPrivateProjections: true, TestOnly: true, TLSRootCAFile: ca}
	if err := os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)
	owner := client(t, s)
	code, row := owner.call("POST", "/v1/intents", "public-authority-intent", input)
	if code != 201 {
		t.Fatal(code, row)
	}
	id := row["id"].(string)
	t.Cleanup(func() {
		code, out := owner.versioned("PATCH", "/v1/intents/"+id, "retire-public-authority", row["etag"].(string), map[string]any{"state": "withdrawn"})
		if code != 200 {
			t.Errorf("retire authority fixture %d %v", code, out)
		}
	})
	runner := New(workerDSN, "development", nil)
	var err error
	runner.semantic, err = semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	revoke := func() {
		switch change {
		case "removed":
			t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", "")
		case "replaced":
			config.ModelRevision = "r2"
		case "public_disabled":
			config.AllowPublicProjections = false
		}
		if change != "removed" {
			if err := os.WriteFile(path, canonical(config), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if inFlight {
		done := make(chan error, 1)
		go func() { done <- runner.scanIntentions(ctx, id) }()
		completed := false
		defer func() {
			unblock()
			cancel()
			if !completed {
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("authority scan did not stop")
				}
			}
		}()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("configured provider never entered", ctx.Err())
		}
		revoke()
		unblock()
		select {
		case err = <-done:
			completed = true
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	} else {
		revoke()
		err = runner.scanIntentions(ctx, id)
	}
	if err != nil {
		t.Fatal("policy withdrawal should decline the old observation", err)
	}
	expectedCalls := int64(0)
	if inFlight {
		expectedCalls = 1
	}
	if calls.Load() != expectedCalls {
		t.Errorf("revoked startup profile reached provider: got %d want %d", calls.Load(), expectedCalls)
	}
	var embeddings, opportunities int
	if err := db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1 AND embedding IS NOT NULL),(SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 OR candidate_intent_id=$1)`, id).Scan(&embeddings, &opportunities); err != nil {
		t.Fatal(err)
	}
	if embeddings != 0 || opportunities != 0 {
		t.Fatalf("revoked public observation persisted: embeddings=%d opportunities=%d", embeddings, opportunities)
	}
}

// Already captured observations are history, not continuing matching authority.
func TestSemanticPublicProfileAuthorityAlreadyEmbedded(t *testing.T) {
	for _, change := range []string{"removed", "replaced", "public_disabled"} {
		t.Run(change, func(t *testing.T) {
			s, admin, workerDSN := semanticFixture(t)
			t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
			summaries := []string{"Environmental regulation reference catalog " + NewID(), "Climate agency citation directory " + NewID()}
			var calls atomic.Int64
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct {
					Model    string `json:"model"`
					Revision string `json:"model_revision"`
					Input    string `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || (request.Input != summaries[0] && request.Input != summaries[1]) {
					http.Error(w, "unapproved projection", 422)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"model": request.Model, "model_revision": request.Revision, "input_sha256": digest([]byte(request.Input)), "embedding": []float64{1, 0, 0}})
			}))
			defer provider.Close()
			dir := t.TempDir()
			ca, path := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "semantic.json")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			config := semanticConfig{Endpoint: provider.URL, ProviderID: "embedded-authority-fixture", Model: "controlled", ModelRevision: "r1", Dimensions: 3, PermissionEvidence: "controlled fixture only", PrivacyPolicySHA256: strings.Repeat("e", 64), AllowPublicProjections: true, AllowPrivateProjections: true, TestOnly: true, TLSRootCAFile: ca}
			if err := os.WriteFile(path, canonical(config), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)
			ids := []string{}
			for _, summary := range summaries {
				owner := client(t, s)
				input := intentFixture()
				input.Visibility = "redacted_public"
				input.PublicSummary = summary
				input.Objective = "Private goal " + NewID()
				code, row := owner.call("POST", "/v1/intents", "embedded-authority-create", input)
				if code != 201 {
					t.Fatal(code, row)
				}
				id, etag := row["id"].(string), row["etag"].(string)
				ids = append(ids, id)
				t.Cleanup(func() {
					code, out := owner.versioned("PATCH", "/v1/intents/"+id, "retire-embedded-authority", etag, map[string]any{"state": "withdrawn"})
					if code != 200 {
						t.Errorf("retire %d %v", code, out)
					}
				})
			}
			runner := New(workerDSN, "development", nil)
			var err error
			runner.semantic, err = semanticConfiguration("development")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			worker, err := pgx.Connect(ctx, workerDSN)
			if err != nil {
				t.Fatal(err)
			}
			defer worker.Close(context.Background())
			if _, err = worker.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if err = runner.ensureSemanticEmbedding(ctx, worker, id); err != nil {
					t.Fatal(err)
				}
			}
			var captured int
			if err = admin.QueryRow(ctx, "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=ANY($1::uuid[]) AND embedding IS NOT NULL", ids).Scan(&captured); err != nil || captured != 2 {
				t.Fatal("capture prerequisite", captured, err)
			}
			switch change {
			case "removed":
				t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", "")
			case "replaced":
				config.ModelRevision = "r2"
			case "public_disabled":
				config.AllowPublicProjections = false
			}
			if change != "removed" {
				if err = os.WriteFile(path, canonical(config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = runner.scanIntentions(ctx, ids[0]); err != nil {
				t.Fatal(err)
			}
			var opportunities, events, inbox int
			if err = admin.QueryRow(ctx, `SELECT
    (SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1),
    (SELECT count(*) FROM undergrid.events e JOIN undergrid.intents i ON i.agent_id=(SELECT agent_id FROM undergrid.intents WHERE id=$1) WHERE i.id=$1 AND e.principal_id=i.principal_id AND e.type='opportunity.available'),
    (SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE b.agent_id=(SELECT agent_id FROM undergrid.intents WHERE id=$1) AND e.type='opportunity.available')`, ids[0]).Scan(&opportunities, &events, &inbox); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 2 {
				t.Errorf("already embedded scan unexpectedly contacted provider: %d", calls.Load())
			}
			if err = admin.QueryRow(ctx, "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=ANY($1::uuid[]) AND embedding IS NOT NULL", ids).Scan(&captured); err != nil || captured != 2 {
				t.Fatal("completed observation history changed", captured, err)
			}
			if opportunities != 0 || events != 0 || inbox != 0 {
				t.Fatalf("revoked profile published semantic effects: opportunities=%d events=%d inbox=%d", opportunities, events, inbox)
			}
		})
	}
}
