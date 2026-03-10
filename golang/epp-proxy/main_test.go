package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
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

func TestEnvOrFirst(t *testing.T) {
	t.Setenv("A_EMPTY", "")
	t.Setenv("B_VAL", "hello")
	if got := envOrFirst([]string{"A_EMPTY", "B_VAL"}, "fallback"); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := dir + "/.env"
	content := "SERVER_PORT=700\nTLS_CLIENT_AUTH=OPTIONAL\n#comment\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write env failed: %v", err)
	}
	_ = os.Unsetenv("SERVER_PORT")
	_ = os.Unsetenv("TLS_CLIENT_AUTH")

	loadDotEnv(envPath)

	if got := os.Getenv("SERVER_PORT"); got != "700" {
		t.Fatalf("expected SERVER_PORT=700 got %q", got)
	}
	if got := os.Getenv("TLS_CLIENT_AUTH"); got != "OPTIONAL" {
		t.Fatalf("expected TLS_CLIENT_AUTH=OPTIONAL got %q", got)
	}
}

func TestParseRateLimitRules(t *testing.T) {
	rules := parseRateLimitRules("10/second,60/minute")
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].limit != 10 || rules[0].window != time.Second {
		t.Fatalf("unexpected first rule: %+v", rules[0])
	}
	if rules[1].limit != 60 || rules[1].window != time.Minute {
		t.Fatalf("unexpected second rule: %+v", rules[1])
	}
}

func TestParseTLSClientAuth(t *testing.T) {
	if got := parseTLSClientAuth("NONE"); got != tls.NoClientCert {
		t.Fatalf("unexpected client auth for NONE: %v", got)
	}
	if got := parseTLSClientAuth("OPTIONAL"); got != tls.VerifyClientCertIfGiven {
		t.Fatalf("unexpected client auth for OPTIONAL: %v", got)
	}
	if got := parseTLSClientAuth("REQUIRE"); got != tls.RequireAndVerifyClientCert {
		t.Fatalf("unexpected client auth for REQUIRE: %v", got)
	}
}

func TestRateLimiterAllow(t *testing.T) {
	limiter := newRateLimiter(Config{})
	cfg := Config{
		IPRateLimitRules: []rateLimitRule{{limit: 2, window: 50 * time.Millisecond}},
		ChannelRateLimit: []rateLimitRule{{limit: 2, window: 50 * time.Millisecond}},
		ReadIPRateLimit:  []rateLimitRule{{limit: 3, window: 50 * time.Millisecond}},
	}

	if !limiter.Allow("1.1.1.1", "", "chan-1", "read", cfg) {
		t.Fatal("first request should pass")
	}
	if !limiter.Allow("1.1.1.1", "", "chan-1", "read", cfg) {
		t.Fatal("second request should pass")
	}
	if limiter.Allow("1.1.1.1", "", "chan-1", "read", cfg) {
		t.Fatal("third request should be blocked")
	}

	time.Sleep(60 * time.Millisecond)
	if !limiter.Allow("1.1.1.1", "", "chan-1", "read", cfg) {
		t.Fatal("request should pass after window reset")
	}
}

func TestRateLimiterAllowWriteAndReadBuckets(t *testing.T) {
	limiter := newRateLimiter(Config{})
	cfg := Config{
		WriteIPRateLimit: []rateLimitRule{{limit: 1, window: time.Second}},
		ReadIPRateLimit:  []rateLimitRule{{limit: 2, window: time.Second}},
	}

	if !limiter.Allow("1.1.1.1", "registrar1", "chan-1", "write", cfg) {
		t.Fatal("first write should pass")
	}
	if limiter.Allow("1.1.1.1", "registrar1", "chan-1", "write", cfg) {
		t.Fatal("second write should be blocked")
	}

	if !limiter.Allow("1.1.1.1", "registrar1", "chan-1", "read", cfg) {
		t.Fatal("first read should pass")
	}
	if !limiter.Allow("1.1.1.1", "registrar1", "chan-1", "read", cfg) {
		t.Fatal("second read should pass")
	}
	if limiter.Allow("1.1.1.1", "registrar1", "chan-1", "read", cfg) {
		t.Fatal("third read should be blocked")
	}
}

func TestRateLimiterAllowReadWriteByIPAndClient(t *testing.T) {
	limiter := newRateLimiter(Config{})
	cfg := Config{
		WriteIPRateLimit:  []rateLimitRule{{limit: 5, window: time.Second}},
		WriteClientLimit:  []rateLimitRule{{limit: 1, window: time.Second}},
		ReadIPRateLimit:   []rateLimitRule{{limit: 2, window: time.Second}},
		ReadClientLimit:   []rateLimitRule{{limit: 2, window: time.Second}},
		ChannelRateLimit:  []rateLimitRule{{limit: 10, window: time.Second}},
		IPRateLimitRules:  []rateLimitRule{{limit: 10, window: time.Second}},
		ClientRateLimit:   []rateLimitRule{{limit: 10, window: time.Second}},
	}

	if !limiter.Allow("1.1.1.1", "user-a", "chan-1", "write", cfg) {
		t.Fatal("first write user-a should pass")
	}
	if limiter.Allow("1.1.1.1", "user-a", "chan-1", "write", cfg) {
		t.Fatal("second write user-a should be blocked by write-client rule")
	}
	if !limiter.Allow("1.1.1.1", "user-b", "chan-2", "write", cfg) {
		t.Fatal("write user-b should pass because client bucket is independent")
	}

	if !limiter.Allow("2.2.2.2", "", "chan-3", "read", cfg) {
		t.Fatal("first anonymous read should pass")
	}
	if !limiter.Allow("2.2.2.2", "", "chan-3", "read", cfg) {
		t.Fatal("second anonymous read should pass")
	}
	if limiter.Allow("2.2.2.2", "", "chan-3", "read", cfg) {
		t.Fatal("third anonymous read should be blocked by read-ip rule")
	}
}

func TestClassifyCommandType(t *testing.T) {
	tests := []struct {
		name   string
		xml    string
		expect string
	}{
		{name: "login", xml: `<epp><command><login/></command></epp>`, expect: "read"},
		{name: "logout", xml: `<epp><command><logout/></command></epp>`, expect: "read"},
		{name: "domain create", xml: `<epp><command><create><domain:create/></create></command></epp>`, expect: "write"},
		{name: "contact update", xml: `<epp><command><update><contact:update/></update></command></epp>`, expect: "write"},
		{name: "host check", xml: `<epp><command><check><host:check/></check></command></epp>`, expect: "read"},
		{name: "domain info", xml: `<epp><command><info><domain:info/></info></command></epp>`, expect: "read"},
		{name: "poll", xml: `<epp><command><poll op="req"/></command></epp>`, expect: "read"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommandType([]byte(tc.xml))
			if got != tc.expect {
				t.Fatalf("expected %q got %q", tc.expect, got)
			}
		})
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

	payload, err := readEPPPayload(bufio.NewReader(client), 1024)
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
	_, err := readEPPPayload(bufio.NewReader(bytes.NewReader(buf.Bytes())), 1024)
	if err == nil {
		t.Fatal("expected invalid frame length error")
	}
}

func TestReadEPPPayloadTooLarge(t *testing.T) {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint32(2048))
	buf.Write(make([]byte, 2044))
	_, err := readEPPPayload(bufio.NewReader(bytes.NewReader(buf.Bytes())), 1024)
	if err == nil {
		t.Fatal("expected frame too large error")
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
