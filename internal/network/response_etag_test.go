package network

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestResponseETagTracksVersionAndOriginalReplay(t *testing.T) {
	s, _ := fixture(t)
	a, b := client(t, s), client(t, s)
	call := func(c testClient, method, path, key, match string, body any) (int, map[string]any, string) {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader(canonical(body)))
		r.Header.Set("Authorization", "Bearer "+c.token)
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("If-Match", match)
		w := httptest.NewRecorder()
		c.s.ServeHTTP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return w.Code, out, w.Header().Get("ETag")
	}
	in := revisionInput("Response header catalog")
	status, created, tag := call(a, "POST", "/v1/intents", "create-header", "", in)
	if status != 201 || tag != `"1"` || created["etag"] != tag {
		t.Fatalf("missing initial ETag: %d %q %#v", status, tag, created)
	}
	id := created["id"].(string)
	status, current, readTag := call(a, "GET", "/v1/intents/"+id, "", "", nil)
	if status != 200 || readTag != tag || current["etag"] != readTag {
		t.Fatalf("read ETag mismatch: %d %q %#v", status, readTag, current)
	}
	status, changed, changedTag := call(a, "PATCH", "/v1/intents/"+id, "edit-header", readTag, map[string]any{"revision": revisionInput("Changed header catalog")})
	if status != 200 || changedTag != `"2"` || changed["etag"] != changedTag {
		t.Fatalf("changed ETag mismatch: %d %q %#v", status, changedTag, changed)
	}
	status, original, replayTag := call(a, "POST", "/v1/intents", "create-header", "", in)
	if status != 201 || original["id"] != id || replayTag != tag || original["etag"] != replayTag {
		t.Fatalf("replay was relabeled with current version: %d %q %#v", status, replayTag, original)
	}
	status, _, errorTag := call(a, "PATCH", "/v1/intents/"+id, "stale-header", tag, map[string]any{"state": "paused"})
	if status != 409 || errorTag != "" {
		t.Fatalf("stale request disclosed resource ETag: %d %q", status, errorTag)
	}
	status, _, errorTag = call(b, "GET", "/v1/intents/"+id, "", "", nil)
	if status != 404 || errorTag != "" {
		t.Fatalf("foreign request disclosed ETag: %d %q", status, errorTag)
	}
}
