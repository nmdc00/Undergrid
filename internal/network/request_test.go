package network

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommandBodyLimitIncludesTrailingBytes(t *testing.T) {
	for _, body := range []string{
		`{"value":"ok"}` + strings.Repeat(" ", 65536),
		`{"value":"ok"}` + strings.Repeat(" ", 65536) + `{"value":"hidden"}`,
	} {
		r := httptest.NewRequest("POST", "/v1/intents", strings.NewReader(body))
		var input struct {
			Value string `json:"value"`
		}
		err := decode(r, &input)
		var p problem
		if !errors.As(err, &p) || p.Status != 413 || p.Code != "body_too_large" {
			t.Fatalf("oversized body accepted or misclassified: %v", err)
		}
	}
}

func TestCommandBodyBoundaryAndShape(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"exact_limit", `{"value":"ok"}` + strings.Repeat(" ", 65536-len(`{"value":"ok"}`)), true},
		{"unknown_field", `{"value":"ok","authority":"spend"}`, false},
		{"second_value", `{"value":"ok"} {}`, false},
		{"truncated", `{"value":`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/intents", strings.NewReader(test.body))
			var input struct {
				Value string `json:"value"`
			}
			err := decode(r, &input)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}
