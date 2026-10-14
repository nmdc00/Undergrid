package network

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNetworkHTTPErrorContract(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("real network database required")
	}
	cases := []struct {
		name, dsn, code string
		status          int
		retryable       bool
	}{
		{"unauthenticated", dsn, "invalid_credential", http.StatusUnauthorized, false},
		{"internal failure", "invalid-test-dsn", "internal_error", http.StatusInternalServerError, true},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for range 2 {
				r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
				r.Header.Set("X-Request-ID", "client-controlled-id")
				r.Header.Set("Authorization", "Bearer ug_secret-network-fixture")
				w := httptest.NewRecorder()
				New(tc.dsn, "development", nil).ServeHTTP(w, r)
				var body struct {
					Error struct {
						Code      string `json:"code"`
						Message   string `json:"message"`
						RequestID string `json:"request_id"`
						Retryable *bool  `json:"retryable"`
					} `json:"error"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				e := body.Error
				if w.Code != tc.status || e.Code != tc.code || e.Message == "" || e.Retryable == nil || *e.Retryable != tc.retryable {
					t.Fatalf("incomplete error contract: %d %s", w.Code, w.Body.String())
				}
				if !validID(e.RequestID) || e.RequestID != w.Header().Get("X-Request-ID") || seen[e.RequestID] {
					t.Fatalf("request ID is absent, untrusted or reused: %q", e.RequestID)
				}
				seen[e.RequestID] = true
				if strings.Contains(w.Body.String(), "invalid-test-dsn") || strings.Contains(w.Body.String(), "secret-network-fixture") {
					t.Fatalf("internal details disclosed: %s", w.Body.String())
				}
			}
		})
	}
}

func TestNetworkErrorRetryGuidance(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		retry  bool
	}{
		{400, "invalid_json", false}, {403, "insufficient_scope", false},
		{404, "not_found", false}, {409, "version_conflict", false},
		{429, "intent_write_rate_exceeded", true}, {503, "artifact_runtime_unavailable", true},
		{501, "unsupported_operation", false},
	} {
		w := httptest.NewRecorder()
		id := NewID()
		w.Header().Set("X-Request-ID", id)
		New("unused", "development", nil).writeProblem(w, fail(tc.status, tc.code))
		var body struct {
			Error struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
				Retryable bool   `json:"retryable"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status || body.Error.Code != tc.code || body.Error.RequestID != id || body.Error.Retryable != tc.retry {
			t.Fatalf("incorrect retry guidance for %s: %d %s", tc.code, w.Code, w.Body.String())
		}
		if tc.status == 429 && w.Header().Get("Retry-After") != "60" {
			t.Fatal("rate limit lost retry delay")
		}
	}
}
