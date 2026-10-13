package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestPrincipalAgentAdmissionQuota(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	body := map[string]any{"name": "another", "scopes": []string{"intents:read"}}
	for i := 0; i < 9; i++ {
		if code, out := a.call("POST", "/v1/agents", "", body); code != 201 {
			t.Fatal(code, out)
		}
	}
	if code, out := a.call("POST", "/v1/agents", "", body); code != 429 {
		t.Fatalf("over-limit creation: %d %v", code, out)
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.agents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&count); err != nil || count != 10 {
		t.Fatal(count, err)
	}
	b := client(t, s)
	if code, out := b.call("POST", "/v1/agents", "", body); code != 201 {
		t.Fatal("other principal", code, out)
	}
}

func TestPrincipalIntentAdmissionConcurrencyAndReplay(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	code, sibling := a.call("POST", "/v1/agents", "", map[string]any{"name": "sibling", "scopes": []string{"intents:write"}})
	if code != 201 {
		t.Fatal(code, sibling)
	}
	b := a
	b.token = sibling["token"].(string)
	body := intentFixture()
	for i := 0; i < 24; i++ {
		if code, out := a.call("POST", "/v1/intents", fmt.Sprint(i), body); code != 201 {
			t.Fatal(code, out)
		}
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := a
			if i == 1 {
				c = b
			}
			code, _ := c.call("POST", "/v1/intents", fmt.Sprintf("race-%d", i), body)
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	success, denied := 0, 0
	for code := range codes {
		if code == 201 {
			success++
		}
		if code == 429 {
			denied++
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("admission race success=%d denied=%d", success, denied)
	}
	if code, out := a.call("POST", "/v1/intents", "0", body); code != 201 {
		t.Fatal("replay at quota", code, out)
	}
	s2 := New(s.dsn, "development", nil)
	a.s = s2
	if code, out := a.call("POST", "/v1/intents", "restart", body); code != 429 {
		t.Fatal("restart quota", code, out)
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&count); err != nil || count != 25 {
		t.Fatal(count, err)
	}
}

func TestPrincipalSwarmAdmissionAcrossAgents(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	body := map[string]any{"name": "coordinator", "scopes": []string{"swarms:create"}}
	_, first := a.call("POST", "/v1/agents", "", body)
	_, second := a.call("POST", "/v1/agents", "", body)
	one, two := a, a
	one.token = first["token"].(string)
	two.token = second["token"].(string)
	room := map[string]any{"objective": "private quota test", "visibility": "private", "recruitment_roles": []any{}}
	for i := 0; i < 10; i++ {
		c := one
		if i%2 == 1 {
			c = two
		}
		if code, out := c.call("POST", "/v1/swarms", fmt.Sprint(i), room); code != 201 {
			t.Fatal(code, out)
		}
	}
	if code, out := one.call("POST", "/v1/swarms", "overflow", room); code != 429 {
		t.Fatal("same-principal private rooms bypass quota", code, out)
	}
	if code, out := two.call("POST", "/v1/swarms", "1", room); code != 201 {
		t.Fatal("replay", code, out)
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.swarms WHERE creator_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&count); err != nil || count != 10 {
		t.Fatal(count, err)
	}
}

func TestPrincipalIntentQuotaPauseAndReactivation(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	body := intentFixture()
	_, created := a.call("POST", "/v1/intents", "first", body)
	id := created["id"].(string)
	for i := 1; i < 25; i++ {
		if code, out := a.call("POST", "/v1/intents", fmt.Sprint(i), body); code != 201 {
			t.Fatal(code, out)
		}
	}
	change := func(key, etag, state string) (int, map[string]any) {
		r := httptest.NewRequest("PATCH", "/v1/intents/"+id, bytes.NewReader(canonical(map[string]string{"state": state})))
		r.Header.Set("Authorization", "Bearer "+a.token)
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("If-Match", etag)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	if code, out := change("pause", created["etag"].(string), "paused"); code != 200 {
		t.Fatal(code, out)
	}
	if code, out := a.call("POST", "/v1/intents", "replacement", body); code != 201 {
		t.Fatal(code, out)
	}
	if code, out := change("resume", "\"2\"", "active"); code != 429 {
		t.Fatal("reactivation bypass", code, out)
	}
	var state string
	var version, events int
	if err := db.QueryRow(context.Background(), "SELECT state,version,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1) FROM undergrid.intents WHERE id=$1", id).Scan(&state, &version, &events); err != nil || state != "paused" || version != 2 || events != 2 {
		t.Fatal(state, version, events, err)
	}
}
