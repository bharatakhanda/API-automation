package main

import "testing"

func TestLoginPayloadUsesVersionSpecificAPIKeyField(t *testing.T) {
	cfg := config{Username: "admin", Password: "password", Secret: "secret"}
	v5 := loginPayload(cfg, apiV5)
	if v5["apikey"] != "secret" || v5["accessrights"] != "" {
		t.Fatalf("unexpected v5 payload: %#v", v5)
	}
	v4 := loginPayload(cfg, apiV4)
	if v4["accessrights"] != "secret" || v4["apikey"] != "" {
		t.Fatalf("unexpected v4 payload: %#v", v4)
	}
}

func TestNormalizeCookieHeader(t *testing.T) {
	for input, want := range map[string]string{
		"_session_id=abc":         "_session_id=abc",
		"Cookie: _session_id=abc": "_session_id=abc",
		"abc":                     "_session_id=abc",
		"":                        "",
	} {
		if got := normalizeCookieHeader(input); got != want {
			t.Errorf("normalizeCookieHeader(%q) = %q, want %q", input, got, want)
		}
	}
}
