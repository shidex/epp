package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	if got := parseTLSClientAuth("OPTIONAL"); got != tls.RequestClientCert {
		t.Fatalf("unexpected client auth for OPTIONAL: %v", got)
	}
	if got := parseTLSClientAuth("REQUIRE"); got != tls.RequireAnyClientCert {
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
		WriteIPRateLimit: []rateLimitRule{{limit: 5, window: time.Second}},
		WriteClientLimit: []rateLimitRule{{limit: 1, window: time.Second}},
		ReadIPRateLimit:  []rateLimitRule{{limit: 2, window: time.Second}},
		ReadClientLimit:  []rateLimitRule{{limit: 2, window: time.Second}},
		ChannelRateLimit: []rateLimitRule{{limit: 10, window: time.Second}},
		IPRateLimitRules: []rateLimitRule{{limit: 10, window: time.Second}},
		ClientRateLimit:  []rateLimitRule{{limit: 10, window: time.Second}},
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

func TestRateLimiterMaxKeys(t *testing.T) {
	limiter := newRateLimiter(Config{RateLimitMaxKeys: 1})
	cfg := Config{IPRateLimitRules: []rateLimitRule{{limit: 10, window: time.Second}}}

	if !limiter.Allow("1.1.1.1", "", "chan-1", "read", cfg) {
		t.Fatal("first key should pass")
	}
	if limiter.Allow("2.2.2.2", "", "chan-2", "read", cfg) {
		t.Fatal("second key should be blocked by max keys")
	}
}

func TestRateLimiterAllowWithReason(t *testing.T) {
	limiter := newRateLimiter(Config{})
	cfg := Config{IPRateLimitRules: []rateLimitRule{{limit: 1, window: time.Second}}}

	if ok, reason := limiter.AllowWithReason("1.1.1.1", "", "chan-1", "read", cfg); !ok || reason != "" {
		t.Fatalf("first call should pass, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := limiter.AllowWithReason("1.1.1.1", "", "chan-1", "read", cfg); ok || reason != "ip" {
		t.Fatalf("second call should fail on ip scope, got ok=%v reason=%q", ok, reason)
	}
}

func TestConnectionTrackerSnapshot(t *testing.T) {
	tracker := newConnectionTracker()
	tracker.connectionOpened("1.1.1.1")
	tracker.attachUsername("user-a")
	tracker.recordCommand("1.1.1.1", "user-a", "read")
	tracker.recordCommand("1.1.1.1", "user-a", "write")
	tracker.recordBlocked("1.1.1.1", "user-a")

	s := getInternalRealtimeStats(tracker)
	if s.Connections.Total != 1 {
		t.Fatalf("expected active total 1 got %d", s.Connections.Total)
	}
	if s.Connections.PerIP["1.1.1.1"] != 1 || s.Connections.PerUser["user-a"] != 1 {
		t.Fatalf("unexpected active snapshots: %+v", s.Connections)
	}
	if s.Commands.TotalRead != 1 || s.Commands.TotalWrite != 1 {
		t.Fatalf("unexpected command totals: %+v", s.Commands)
	}
	if s.Blocked.Total != 1 || s.Blocked.PerIP["1.1.1.1"] != 1 || s.Blocked.PerUser["user-a"] != 1 {
		t.Fatalf("unexpected blocked stats: %+v", s.Blocked)
	}

	tracker.detachUsername("user-a")
	tracker.connectionClosed("1.1.1.1")
	s = getInternalRealtimeStats(tracker)
	if s.Connections.Total != 0 {
		t.Fatalf("expected active total 0 got %d", s.Connections.Total)
	}
	if _, ok := s.Connections.PerIP["1.1.1.1"]; ok {
		t.Fatalf("expected ip key deleted: %+v", s.Connections.PerIP)
	}
	if _, ok := s.Connections.PerUser["user-a"]; ok {
		t.Fatalf("expected user key deleted: %+v", s.Connections.PerUser)
	}
}

func TestLogEventJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(buf, "", 0)
	logEvent(logger, "json", "info", "sample", map[string]any{"channel": "c1"})

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte(`"event":"sample"`)) {
		t.Fatalf("unexpected log output: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte(`"channel":"c1"`)) {
		t.Fatalf("missing fields in log output: %s", out)
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
		var req authRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode auth request: %v", err)
		}
		if req.ServerCertificateHash != "cert-hash" {
			t.Fatalf("unexpected serverCertificateHash: %q", req.ServerCertificateHash)
		}
		if req.HashCertificate != "cert-hash" {
			t.Fatalf("unexpected hashCertificate: %q", req.HashCertificate)
		}
		if req.ClientCertificate != "client-cert-pem" {
			t.Fatalf("unexpected clientCertificate: %q", req.ClientCertificate)
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
	token, ok := processAuthorization(httpClient, authSrv.URL, "1.1.1.1", loginXML{ClientID: "u", Password: "p"}, "cert-hash", "client-cert-pem", 1024)
	if !ok || token != "tok-1" {
		t.Fatalf("unexpected auth result ok=%v token=%q", ok, token)
	}

	resp, err := postEPPCommand(httpClient, cmdSrv.URL, token, []byte("<epp/>"), 1024)
	if err != nil {
		t.Fatalf("postEPPCommand failed: %v", err)
	}
	if !bytes.Contains(resp, []byte(`code="1000"`)) {
		t.Fatalf("unexpected command response: %s", string(resp))
	}
}

func TestProcessAuthorizationIncludesCertificateHashFieldWhenEmpty(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode auth request: %v", err)
		}
		v, ok := req["serverCertificateHash"]
		if !ok {
			t.Fatal("expected serverCertificateHash field to be present")
		}
		if got, ok := v.(string); !ok || got != "" {
			t.Fatalf("unexpected serverCertificateHash value: %#v", v)
		}
		legacy, ok := req["hashCertificate"]
		if !ok {
			t.Fatal("expected hashCertificate field to be present")
		}
		if got, ok := legacy.(string); !ok || got != "" {
			t.Fatalf("unexpected hashCertificate value: %#v", legacy)
		}
		cert, ok := req["clientCertificate"]
		if !ok {
			t.Fatal("expected clientCertificate field to be present")
		}
		if got, ok := cert.(string); !ok || got != "" {
			t.Fatalf("unexpected clientCertificate value: %#v", cert)
		}
		_, _ = w.Write([]byte(`{"responseCode":"00","eppSessionToken":"tok-1"}`))
	}))
	defer authSrv.Close()

	httpClient := &http.Client{Timeout: time.Second}
	token, ok := processAuthorization(httpClient, authSrv.URL, "1.1.1.1", loginXML{ClientID: "u", Password: "p"}, "", "", 1024)
	if !ok || token != "tok-1" {
		t.Fatalf("unexpected auth result ok=%v token=%q", ok, token)
	}
}

func TestPostEPPCommandStatusAndSizeLimit(t *testing.T) {
	statusSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer statusSrv.Close()

	httpClient := &http.Client{Timeout: time.Second}
	_, err := postEPPCommand(httpClient, statusSrv.URL, "tok", []byte("<epp/>"), 1024)
	if err == nil {
		t.Fatal("expected error when backend returns non-2xx")
	}

	largeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("123456"))
	}))
	defer largeSrv.Close()

	_, err = postEPPCommand(httpClient, largeSrv.URL, "tok", []byte("<epp/>"), 5)
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestBuildResponsesEscapeXML(t *testing.T) {
	login := buildLoginResponse(`<tag>&"`)
	if bytes.Contains([]byte(login), []byte(`<clTRID><tag>&"</clTRID>`)) {
		t.Fatal("clTRID should be XML escaped")
	}

	errResp := buildErrorResponse(`<oops>&`)
	if bytes.Contains([]byte(errResp), []byte(`<msg><oops>&</msg>`)) {
		t.Fatal("error message should be XML escaped")
	}

	logout := buildLogoutResponse(`<bye>&`)
	if bytes.Contains([]byte(logout), []byte(`<clTRID><bye>&</clTRID>`)) {
		t.Fatal("logout clTRID should be XML escaped")
	}
}

func TestReadBodyWithLimit(t *testing.T) {
	got, err := readBodyWithLimit(bytes.NewBufferString("abc"), 3)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if string(got) != "abc" {
		t.Fatalf("unexpected body: %q", string(got))
	}

	_, err = readBodyWithLimit(bytes.NewBufferString("abcd"), 3)
	if err == nil {
		t.Fatal("expected limit exceeded error")
	}

	_, err = readBodyWithLimit(errReader{}, 3)
	if err == nil {
		t.Fatal("expected reader error")
	}
}

func TestWriteJSONWithTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	payload := []byte(`{"ok":true}`)

	if err := writeJSONWithTimeout(path, payload, time.Second); err != nil {
		t.Fatalf("writeJSONWithTimeout failed: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if got := string(raw); !strings.Contains(got, `{"ok":true}`) {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestStartRealtimeStatsWriterCreatesFile(t *testing.T) {
	dir := t.TempDir()
	statsPath := filepath.Join(dir, "realtime", "stats.json")
	tracker := newConnectionTracker()
	tracker.connectionOpened("1.1.1.1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := log.New(io.Discard, "", 0)
	stop := startRealtimeStatsWriter(ctx, logger, Config{
		RealtimeStatsFile:         statsPath,
		RealtimeStatsInterval:     20 * time.Millisecond,
		RealtimeStatsWriteTimeout: 200 * time.Millisecond,
		LogFormat:                 "json",
	}, tracker)
	defer stop()

	deadline := time.Now().Add(time.Second)
	for {
		raw, err := os.ReadFile(statsPath)
		if err == nil {
			if strings.Contains(string(raw), `"connections"`) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("realtime stats file was not written in time: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("read failed")
}

func TestNewBackendHTTPClient(t *testing.T) {
	cfg := Config{
		BackendTimeout:             9 * time.Second,
		BackendDialTimeout:         2 * time.Second,
		BackendTLSHandshake:        3 * time.Second,
		BackendIdleConnTimeout:     11 * time.Second,
		BackendMaxIdleConns:        321,
		BackendMaxIdleConnsPerHost: 123,
		BackendMaxConnsPerHost:     99,
	}

	httpClient := newBackendHTTPClient(cfg)
	if httpClient.Timeout != 9*time.Second {
		t.Fatalf("unexpected backend timeout: %v", httpClient.Timeout)
	}

	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", httpClient.Transport)
	}
	if transport.MaxIdleConns != 321 {
		t.Fatalf("unexpected MaxIdleConns: %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 123 {
		t.Fatalf("unexpected MaxIdleConnsPerHost: %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 99 {
		t.Fatalf("unexpected MaxConnsPerHost: %d", transport.MaxConnsPerHost)
	}
	if transport.IdleConnTimeout != 11*time.Second {
		t.Fatalf("unexpected IdleConnTimeout: %v", transport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != 3*time.Second {
		t.Fatalf("unexpected TLSHandshakeTimeout: %v", transport.TLSHandshakeTimeout)
	}
}

func TestResolveRegistrarCertificateHashNonTLSConn(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	hash, err := resolveRegistrarCertificateHash(serverConn)
	if err != nil {
		t.Fatalf("resolveRegistrarCertificateHash returned error for non-TLS conn: %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash for non-TLS conn, got %q", hash)
	}
}

func TestResolveRegistrarCertificateHashUsesClientCertificateSHA1(t *testing.T) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key failed: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(91),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert failed: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert failed: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key failed: %v", err)
	}
	serverTpl := &x509.Certificate{
		SerialNumber: big.NewInt(92),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert failed: %v", err)
	}
	serverTLSCert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}),
	)
	if err != nil {
		t.Fatalf("load server key pair failed: %v", err)
	}

	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client key failed: %v", err)
	}
	clientTpl := &x509.Certificate{
		SerialNumber: big.NewInt(93),
		Subject:      pkix.Name{CommonName: "registrar-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client cert failed: %v", err)
	}
	clientTLSCert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)}),
	)
	if err != nil {
		t.Fatalf("load client key pair failed: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	})
	if err != nil {
		t.Fatalf("tls listen failed: %v", err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			errCh <- acceptErr
			return
		}
		defer conn.Close()
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			errCh <- fmt.Errorf("expected *tls.Conn, got %T", conn)
			return
		}
		if hsErr := tlsConn.Handshake(); hsErr != nil {
			errCh <- hsErr
			return
		}
		hash, hashErr := resolveRegistrarCertificateHash(tlsConn)
		if hashErr != nil {
			errCh <- hashErr
			return
		}
		expectedRaw := clientDER
		sum := sha1.Sum(expectedRaw)
		expectedHash := strings.ToUpper(hex.EncodeToString(sum[:]))
		if hash != expectedHash {
			errCh <- fmt.Errorf("unexpected hash: got %q want %q", hash, expectedHash)
			return
		}
		errCh <- nil
	}()

	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		Certificates:       []tls.Certificate{clientTLSCert},
		RootCAs:            caPool,
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	_ = conn.Close()

	if err := <-errCh; err != nil {
		t.Fatalf("server validation failed: %v", err)
	}
}
func TestBuildListenerTLSSkipsCAWhenClientAuthNone(t *testing.T) {
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate root key failed: %v", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(11),
		Subject:               pkix.Name{CommonName: "root-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create root cert failed: %v", err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("parse root cert failed: %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate leaf key failed: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(12),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create leaf cert failed: %v", err)
	}

	dir := t.TempDir()
	certPath := dir + "/server.pem"
	keyPath := dir + "/server.key"

	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0o644); err != nil {
		t.Fatalf("write cert failed: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}), 0o600); err != nil {
		t.Fatalf("write key failed: %v", err)
	}

	ln, err := buildListener(Config{
		ListenAddr:    "127.0.0.1:0",
		FrontendTLS:   true,
		FrontendCert:  certPath,
		FrontendKey:   keyPath,
		FrontendCA:    dir + "/missing-ca.pem",
		TLSClientAuth: tls.NoClientCert,
	})
	if err != nil {
		t.Fatalf("build TLS listener should ignore CA when client auth NONE: %v", err)
	}
	defer ln.Close()
}

func TestBuildListenerTLSWithFullChainCertificate(t *testing.T) {
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate root key failed: %v", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "root-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create root cert failed: %v", err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("parse root cert failed: %v", err)
	}

	intermediateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate intermediate key failed: %v", err)
	}
	intermediateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "intermediate-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	intermediateDER, err := x509.CreateCertificate(rand.Reader, intermediateTemplate, rootCert, &intermediateKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create intermediate cert failed: %v", err)
	}
	intermediateCert, err := x509.ParseCertificate(intermediateDER)
	if err != nil {
		t.Fatalf("parse intermediate cert failed: %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate leaf key failed: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, intermediateCert, &leafKey.PublicKey, intermediateKey)
	if err != nil {
		t.Fatalf("create leaf cert failed: %v", err)
	}

	dir := t.TempDir()
	certPath := dir + "/server-fullchain.pem"
	keyPath := dir + "/server.key"
	caPath := dir + "/client-ca.pem"

	certPEM := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: intermediateDER})...,
	)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write fullchain cert failed: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}), 0o600); err != nil {
		t.Fatalf("write private key failed: %v", err)
	}
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), 0o644); err != nil {
		t.Fatalf("write root ca failed: %v", err)
	}

	ln, err := buildListener(Config{
		ListenAddr:    "127.0.0.1:0",
		FrontendTLS:   true,
		FrontendCert:  certPath,
		FrontendKey:   keyPath,
		FrontendCA:    caPath,
		TLSClientAuth: tls.NoClientCert,
	})
	if err != nil {
		t.Fatalf("build TLS listener failed: %v", err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			errCh <- acceptErr
			return
		}
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			_ = conn.Close()
			errCh <- nil
			return
		}
		handshakeErr := tlsConn.Handshake()
		_ = tlsConn.Close()
		errCh <- handshakeErr
	}()

	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})) {
		t.Fatal("failed appending root cert")
	}

	clientConn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: rootPool, ServerName: "localhost", MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("TLS dial to listener failed: %v", err)
	}
	state := clientConn.ConnectionState()
	_ = clientConn.Close()

	if len(state.PeerCertificates) < 2 {
		t.Fatalf("expected fullchain sent by server, got %d certificates", len(state.PeerCertificates))
	}
	if state.PeerCertificates[1].Subject.CommonName != "intermediate-ca" {
		t.Fatalf("expected intermediate in peer chain, got %q", state.PeerCertificates[1].Subject.CommonName)
	}

	if acceptErr := <-errCh; acceptErr != nil {
		t.Fatalf("accept failed: %v", acceptErr)
	}
}
