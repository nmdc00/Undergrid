package identity

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOwnerSessionRequiresAuthenticatedSameOriginAction(t *testing.T) {
	a, err := NewSessionManager("https://undergrid.test", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	record := session{Issuer: "https://issuer.test", Subject: "owner-a", CSRF: "unpredictable-csrf", Expires: time.Now().Add(time.Minute).Unix()}
	w := httptest.NewRecorder()
	if err = a.writeCookie(w, ownerCookie, record, 900); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatal("unsafe cookie attributes")
	}
	request := func(origin, csrf string) bool {
		r := httptest.NewRequest("POST", "https://undergrid.test/v1/agents", nil)
		r.AddCookie(cookie)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrf)
		_, err := a.Authenticate(r)
		return err == nil
	}
	if request("https://attacker.test", record.CSRF) || request("https://undergrid.test", "wrong") {
		t.Fatal("cross-origin or missing CSRF authority accepted")
	}
	if !request("https://undergrid.test", record.CSRF) {
		t.Fatal("valid interactive session rejected")
	}
	cookie.Value = cookie.Value[:len(cookie.Value)-4] + "AAAA"
	if request("https://undergrid.test", record.CSRF) {
		t.Fatal("tampered session accepted")
	}
	record.Expires = time.Now().Add(-time.Second).Unix()
	w = httptest.NewRecorder()
	_ = a.writeCookie(w, ownerCookie, record, 900)
	cookie = w.Result().Cookies()[0]
	if request("https://undergrid.test", record.CSRF) {
		t.Fatal("expired session accepted")
	}
}

func TestSessionConfigurationFailsClosed(t *testing.T) {
	for _, origin := range []string{"http://undergrid.test", "https://user:pass@undergrid.test", "https://undergrid.test/path"} {
		if _, err := NewSessionManager(origin, bytes.Repeat([]byte{1}, 32)); err == nil {
			t.Fatalf("unsafe origin %s", origin)
		}
	}
	if _, err := NewSessionManager("https://undergrid.test", []byte("short")); err == nil {
		t.Fatal("weak key accepted")
	}
}
