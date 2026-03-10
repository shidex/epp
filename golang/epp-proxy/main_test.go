package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_ENV_OR", "value")
	if got := envOr("TEST_ENV_OR", "fallback"); got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}

	_ = os.Unsetenv("TEST_ENV_OR")
	if got := envOr("TEST_ENV_OR", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback value, got %q", got)
	}
}

func TestBoolFromEnv(t *testing.T) {
	t.Setenv("TEST_BOOL", "true")
	if !boolFromEnv("TEST_BOOL", false) {
		t.Fatal("expected true")
	}

	t.Setenv("TEST_BOOL", "no")
	if boolFromEnv("TEST_BOOL", true) {
		t.Fatal("expected false")
	}

	_ = os.Unsetenv("TEST_BOOL")
	if !boolFromEnv("TEST_BOOL", true) {
		t.Fatal("expected fallback true")
	}
}

func TestDurationFromEnv(t *testing.T) {
	t.Setenv("TEST_DURATION", "7s")
	if got := durationFromEnv("TEST_DURATION", 2*time.Second); got != 7*time.Second {
		t.Fatalf("expected 7s, got %v", got)
	}

	t.Setenv("TEST_DURATION", "invalid")
	if got := durationFromEnv("TEST_DURATION", 2*time.Second); got != 2*time.Second {
		t.Fatalf("expected fallback 2s, got %v", got)
	}
}

func TestIntFromEnv(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := intFromEnv("TEST_INT", 10); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}

	t.Setenv("TEST_INT", "bad")
	if got := intFromEnv("TEST_INT", 10); got != 10 {
		t.Fatalf("expected fallback 10, got %d", got)
	}
}

func TestRateLimiterAllow(t *testing.T) {
	limiter := &rateLimiter{max: 2, window: 50 * time.Millisecond, buckets: make(map[string]bucket)}
	if !limiter.Allow("1.1.1.1", "", "ip") {
		t.Fatal("first request should pass")
	}
	if !limiter.Allow("1.1.1.1", "", "ip") {
		t.Fatal("second request should pass")
	}
	if limiter.Allow("1.1.1.1", "", "ip") {
		t.Fatal("third request should be blocked")
	}

	time.Sleep(60 * time.Millisecond)
	if !limiter.Allow("1.1.1.1", "", "ip") {
		t.Fatal("request should pass after window reset")
	}
}

func TestRateLimiterKey(t *testing.T) {
	limiter := &rateLimiter{}
	if got := limiter.Key("2.2.2.2", "alice", "username"); got != "user:alice" {
		t.Fatalf("unexpected username key: %s", got)
	}
	if got := limiter.Key("2.2.2.2", "", "username"); got != "ip:2.2.2.2" {
		t.Fatalf("unexpected username fallback key: %s", got)
	}
	if got := limiter.Key("2.2.2.2", "alice", "ip"); got != "ip:2.2.2.2" {
		t.Fatalf("unexpected ip key: %s", got)
	}
	if got := limiter.Key("2.2.2.2", "alice", "ip_or_username"); got != "user:alice" {
		t.Fatalf("unexpected default key: %s", got)
	}
}

func TestParseLoginXML(t *testing.T) {
	xmlPayload := []byte(`<?xml version="1.0" encoding="UTF-8"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><command><login><clID>registrar1</clID><pw>pw1</pw><newPW>pw2</newPW></login><clTRID>abc</clTRID></command></epp>`)
	got, err := parseLoginXML(xmlPayload)
	if err != nil {
		t.Fatalf("parse login failed: %v", err)
	}
	if got.ClientID != "registrar1" || got.Password != "pw1" || got.NewPassword != "pw2" || got.ClTRID != "abc" {
		t.Fatalf("unexpected parsed login: %+v", got)
	}
}

func TestBuildRateLimitExceededResponse(t *testing.T) {
	resp := string(buildRateLimitExceededResponse())
	if !bytes.Contains([]byte(resp), []byte(`code="2400"`)) {
		t.Fatalf("expected result code 2400 in response: %s", resp)
	}
	if !bytes.Contains([]byte(resp), []byte("Rate limit exceeded")) {
		t.Fatalf("expected limit exceeded message in response: %s", resp)
	}
}

func TestReadWriteEPPPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- writeEPPPayload(server, []byte("<epp/>"))
	}()

	payload, err := readEPPPayload(bufio.NewReader(client))
	if err != nil {
		t.Fatalf("readEPPPayload failed: %v", err)
	}
	if string(payload) != "<epp/>" {
		t.Fatalf("unexpected payload: %q", string(payload))
	}
	if err = <-errCh; err != nil {
		t.Fatalf("writeEPPPayload failed: %v", err)
	}
}

func TestReadEPPPayloadInvalidLength(t *testing.T) {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(4))
	_, err := readEPPPayload(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err == nil {
		t.Fatal("expected invalid frame length error")
	}
}

func TestProcessAuthorizationAndCommand(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authentication") != "" {
			t.Fatalf("expected empty authentication header for auth")
		}
		_, _ = w.Write([]byte(`{"responseCode":"00","eppSessionToken":"tok-1"}`))
	}))
	defer authSrv.Close()

	cmdSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authentication") != "tok-1" {
			t.Fatalf("missing expected token header")
		}
		_, _ = w.Write([]byte(`<epp><response><result code="1000"/></response></epp>`))
	}))
	defer cmdSrv.Close()

	httpClient := &http.Client{Timeout: time.Second}
	token, ok := processAuthorization(httpClient, authSrv.URL, "1.1.1.1", loginXML{ClientID: "u", Password: "p"})
	if !ok || token != "tok-1" {
		t.Fatalf("unexpected auth result ok=%v token=%q", ok, token)
	}

	resp, err := postEPPCommand(httpClient, cmdSrv.URL, token, []byte("<epp/>"))
	if err != nil {
		t.Fatalf("postEPPCommand failed: %v", err)
	}
	if !bytes.Contains(resp, []byte(`code="1000"`)) {
		t.Fatalf("unexpected command response: %s", string(resp))
	}
}
