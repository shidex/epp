package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ListenAddr        string
	ConnectTimeout    time.Duration
	IdleTimeout       time.Duration
	WriteTimeout      time.Duration
	FrontendTLS       bool
	FrontendCert      string
	FrontendKey       string
	FrontendCA        string
	TLSClientAuth     tls.ClientAuthType
	AuthBackendURL    string
	CommandBackendURL string
	LogoutBackendURL  string
	IPRateLimitRules  []rateLimitRule
	ClientRateLimit   []rateLimitRule
	ChannelRateLimit  []rateLimitRule
	WriteRateLimit    []rateLimitRule
	ReadRateLimit     []rateLimitRule
	ReadIPRateLimit   []rateLimitRule
	WriteIPRateLimit  []rateLimitRule
	ReadClientLimit   []rateLimitRule
	WriteClientLimit  []rateLimitRule
	MaxFrameSize      int
	MaxConns          int
	RateLimitMaxKeys  int
	LogFormat         string
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]map[string][]bucket
	maxKeys int
}

type bucket struct {
	windowStart time.Time
	count       int
}

type rateLimitRule struct {
	limit  int
	window time.Duration
}

type authRequest struct {
	EppUsername           string `json:"eppUsername,omitempty"`
	EppPassword           string `json:"eppPassword,omitempty"`
	EppNewPassword        string `json:"eppNewPassword,omitempty"`
	ServerCertificateHash string `json:"serverCertificateHash,omitempty"`
	IPAddress             string `json:"ipAddress,omitempty"`
}

type authResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseDesc    string `json:"responseDesc"`
	EppSessionToken string `json:"eppSessionToken"`
}

type loginXML struct {
	ClientID    string `xml:"command>login>clID"`
	Password    string `xml:"command>login>pw"`
	NewPassword string `xml:"command>login>newPW"`
	ClTRID      string `xml:"command>clTRID"`
}

func main() {
	cfg := loadConfig()
	logger := log.New(os.Stdout, "[go-epp-proxy] ", log.LstdFlags|log.Lmicroseconds)

	ln, err := buildListener(cfg)
	if err != nil {
		logger.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	limiter := newRateLimiter(cfg)
	connSlots := make(chan struct{}, max(1, cfg.MaxConns))

	logEvent(logger, cfg.LogFormat, "info", "service_started", map[string]any{"listen_addr": cfg.ListenAddr, "auth_url": cfg.AuthBackendURL, "command_url": cfg.CommandBackendURL, "max_conns": cfg.MaxConns})

	var wg sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				logger.Printf("temporary accept error: %v", err)
				continue
			}
			logger.Printf("accept error: %v", err)
			continue
		}

		wg.Add(1)
		select {
		case connSlots <- struct{}{}:
		default:
			logEvent(logger, cfg.LogFormat, "warn", "connection_rejected", map[string]any{"remote_addr": conn.RemoteAddr().String(), "reason": "max_conns_reached"})
			_ = conn.Close()
			wg.Done()
			continue
		}
		go func(client net.Conn) {
			defer wg.Done()
			defer func() { <-connSlots }()
			handleConn(cfg, logger, limiter, client)
		}(conn)
	}

	logEvent(logger, cfg.LogFormat, "info", "service_stopping", nil)
	_ = ln.Close()
	wg.Wait()
}

func loadConfig() Config {
	envFile := discoverEnvFile()
	loadDotEnv(envFile)

	listen := flag.String("listen", resolveAddr(envOrFirst([]string{"SERVER_PORT", "EPP_SERVER_PORT", "EPP_LISTEN_ADDR"}, "700")), "address to listen on")
	connectTimeout := flag.Duration("connect-timeout", durationWithFallback(envOr("EPP_CONNECT_TIMEOUT", "5s"), 5*time.Second), "backend connect timeout")
	idleTimeout := flag.Duration("idle-timeout", durationWithFallback(envOrFirst([]string{"IDLE_TIMEOUT_SECONDS", "EPP_IDLE_TIMEOUT"}, "600"), 10*time.Minute), "idle timeout for client connection")
	writeTimeout := flag.Duration("write-timeout", durationWithFallback(envOr("EPP_WRITE_TIMEOUT", "10s"), 10*time.Second), "write timeout for client connection")
	frontendTLS := flag.Bool("frontend-tls", boolWithFallback(envOrFirst([]string{"SERVER_SSL_ENABLED", "EPP_FRONTEND_TLS"}, "false"), false), "enable TLS listener")
	frontendCert := flag.String("frontend-cert", envOrFirst([]string{"TLS_SERVER_CERT", "EPP_FRONTEND_CERT"}, "certs/server.crt"), "TLS cert path")
	frontendKey := flag.String("frontend-key", envOrFirst([]string{"TLS_SERVER_KEY", "EPP_FRONTEND_KEY"}, "certs/server.key"), "TLS key path")
	frontendCA := flag.String("frontend-ca", envOrFirst([]string{"TLS_CA_CERT", "EPP_FRONTEND_CA"}, "certs/cacert.pem"), "trusted client CA path")
	tlsClientAuth := flag.String("tls-client-auth", strings.ToUpper(envOrFirst([]string{"TLS_CLIENT_AUTH", "EPP_TLS_CLIENT_AUTH"}, "REQUIRE")), "TLS client certificate mode: NONE, OPTIONAL, REQUIRE")
	authURL := flag.String("auth-url", envOrFirst([]string{"AUTHBACKEND_URL", "EPP_AUTH_URL"}, "http://localhost:8080/PANDI-REGISTRAR-0.1/authRegistrar/"), "backend auth URL")
	commandURL := flag.String("command-url", envOrFirst([]string{"BACKEND_URL", "EPP_COMMAND_URL"}, "http://localhost:8080/PANDI-CORE-0.1/processepp/"), "backend command URL")
	logoutURL := flag.String("logout-url", envOrFirst([]string{"LOGOUTBACKEND_URL", "EPP_LOGOUT_URL"}, "http://localhost:8080/PANDI-REGISTRAR-0.1/logoutRegistrar/"), "backend logout URL")
	rateLimitIP := flag.String("rate-limit-ip", envOrFirst([]string{"RATELIMIT_IP_RULES", "EPP_RATE_LIMIT_IP_RULES"}, "10/second,60/minute"), "rate limit rules by IP")
	rateLimitClient := flag.String("rate-limit-client", envOrFirst([]string{"RATELIMIT_CLIENT_RULES", "EPP_RATE_LIMIT_CLIENT_RULES"}, "50/second,500/minute"), "rate limit rules by client ID")
	rateLimitChannel := flag.String("rate-limit-channel", envOrFirst([]string{"RATELIMIT_CHANNEL_RULES", "EPP_RATE_LIMIT_CHANNEL_RULES"}, "10/second,60/minute"), "rate limit rules by channel")
	rateLimitWrite := flag.String("rate-limit-write", envOrFirst([]string{"RATELIMIT_WRITE_RULES", "EPP_RATE_LIMIT_WRITE_RULES"}, "10/second"), "legacy shared rate limit rules for write commands")
	rateLimitRead := flag.String("rate-limit-read", envOrFirst([]string{"RATELIMIT_READ_RULES", "EPP_RATE_LIMIT_READ_RULES"}, "50/second,500/minute"), "legacy shared rate limit rules for read commands")
	rateLimitReadIP := flag.String("rate-limit-read-ip", envOrFirst([]string{"RATELIMIT_READ_IP_RULES", "EPP_RATE_LIMIT_READ_IP_RULES", "RATELIMIT_READ_RULES", "EPP_RATE_LIMIT_READ_RULES", "RATELIMIT_IP_RULES", "EPP_RATE_LIMIT_IP_RULES"}, "10/second,60/minute"), "rate limit rules for read commands by IP")
	rateLimitWriteIP := flag.String("rate-limit-write-ip", envOrFirst([]string{"RATELIMIT_WRITE_IP_RULES", "EPP_RATE_LIMIT_WRITE_IP_RULES", "RATELIMIT_WRITE_RULES", "EPP_RATE_LIMIT_WRITE_RULES", "RATELIMIT_IP_RULES", "EPP_RATE_LIMIT_IP_RULES"}, "10/second,60/minute"), "rate limit rules for write commands by IP")
	rateLimitReadClient := flag.String("rate-limit-read-client", envOrFirst([]string{"RATELIMIT_READ_CLIENT_RULES", "EPP_RATE_LIMIT_READ_CLIENT_RULES", "RATELIMIT_READ_RULES", "EPP_RATE_LIMIT_READ_RULES", "RATELIMIT_CLIENT_RULES", "EPP_RATE_LIMIT_CLIENT_RULES"}, "50/second,500/minute"), "rate limit rules for read commands by username/client")
	rateLimitWriteClient := flag.String("rate-limit-write-client", envOrFirst([]string{"RATELIMIT_WRITE_CLIENT_RULES", "EPP_RATE_LIMIT_WRITE_CLIENT_RULES", "RATELIMIT_WRITE_RULES", "EPP_RATE_LIMIT_WRITE_RULES", "RATELIMIT_CLIENT_RULES", "EPP_RATE_LIMIT_CLIENT_RULES"}, "50/second,500/minute"), "rate limit rules for write commands by username/client")
	maxFrameSize := flag.Int("max-frame-size", intWithFallback(envOr("EPP_MAX_FRAME_BYTES", "65535"), 65535), "maximum RFC5734 frame size in bytes")
	maxConns := flag.Int("max-conns", intWithFallback(envOr("EPP_MAX_CONNS", "1000"), 1000), "maximum concurrent accepted connections")
	rateLimitMaxKeys := flag.Int("rate-limit-max-keys", intWithFallback(envOr("EPP_RATELIMIT_MAX_KEYS", "100000"), 100000), "maximum tracked keys per rate limiter scope")
	logFormat := flag.String("log-format", strings.ToLower(envOr("EPP_LOG_FORMAT", "json")), "log format: json or text")
	flag.Parse()

	return Config{
		ListenAddr:        *listen,
		ConnectTimeout:    *connectTimeout,
		IdleTimeout:       *idleTimeout,
		WriteTimeout:      *writeTimeout,
		FrontendTLS:       *frontendTLS,
		FrontendCert:      *frontendCert,
		FrontendKey:       *frontendKey,
		FrontendCA:        *frontendCA,
		TLSClientAuth:     parseTLSClientAuth(*tlsClientAuth),
		AuthBackendURL:    *authURL,
		CommandBackendURL: *commandURL,
		LogoutBackendURL:  *logoutURL,
		IPRateLimitRules:  parseRateLimitRules(*rateLimitIP),
		ClientRateLimit:   parseRateLimitRules(*rateLimitClient),
		ChannelRateLimit:  parseRateLimitRules(*rateLimitChannel),
		WriteRateLimit:    parseRateLimitRules(*rateLimitWrite),
		ReadRateLimit:     parseRateLimitRules(*rateLimitRead),
		ReadIPRateLimit:   parseRateLimitRules(*rateLimitReadIP),
		WriteIPRateLimit:  parseRateLimitRules(*rateLimitWriteIP),
		ReadClientLimit:   parseRateLimitRules(*rateLimitReadClient),
		WriteClientLimit:  parseRateLimitRules(*rateLimitWriteClient),
		MaxFrameSize:      *maxFrameSize,
		MaxConns:          *maxConns,
		RateLimitMaxKeys:  *rateLimitMaxKeys,
		LogFormat:         *logFormat,
	}
}

func buildListener(cfg Config) (net.Listener, error) {
	if !cfg.FrontendTLS {
		return net.Listen("tcp", cfg.ListenAddr)
	}

	certificate, err := tls.LoadX509KeyPair(cfg.FrontendCert, cfg.FrontendKey)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{Certificates: []tls.Certificate{certificate}, ClientAuth: cfg.TLSClientAuth, MinVersion: tls.VersionTLS12}

	if cfg.TLSClientAuth != tls.NoClientCert {
		caBytes, err := os.ReadFile(cfg.FrontendCA)
		if err != nil {
			return nil, err
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("invalid CA certificate in %s", cfg.FrontendCA)
		}
		tlsCfg.ClientCAs = caPool
	}

	return tls.Listen("tcp", cfg.ListenAddr, tlsCfg)
}

func handleConn(cfg Config, logger *log.Logger, limiter *rateLimiter, client net.Conn) {
	defer client.Close()
	clientID := client.RemoteAddr().String()
	remoteAddr := remoteIP(client.RemoteAddr())
	httpClient := &http.Client{Timeout: cfg.ConnectTimeout}

	logEvent(logger, cfg.LogFormat, "info", "client_connected", map[string]any{"channel": clientID, "remote_ip": remoteAddr})
	if err := writeEPPPayload(client, []byte(buildGreetingResponse())); err != nil {
		logEvent(logger, cfg.LogFormat, "error", "greeting_failed", map[string]any{"channel": clientID, "error": err.Error()})
		return
	}

	reader := bufio.NewReader(client)
	authenticated := false
	username := ""
	token := ""

	for {
		_ = client.SetReadDeadline(time.Now().Add(cfg.IdleTimeout))
		payload, err := readEPPPayload(reader, cfg.MaxFrameSize)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logEvent(logger, cfg.LogFormat, "warn", "read_failed", map[string]any{"channel": clientID, "error": err.Error()})
			}
			return
		}

		if extracted := extractEPPUsername(payload); extracted != "" {
			username = extracted
		}

		commandType := classifyCommandType(payload)
		if !limiter.Allow(remoteAddr, username, clientID, commandType, cfg) {
			logEvent(logger, cfg.LogFormat, "warn", "rate_limited", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "username": username, "command_type": commandType})
			_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			if err = writeEPPPayload(client, buildRateLimitExceededResponse()); err != nil {
				logEvent(logger, cfg.LogFormat, "error", "write_rate_limit_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
			}
			return
		}

		xmlBody := string(payload)
		switch {
		case !authenticated:
			loginReq, parseErr := parseLoginXML(payload)
			if parseErr != nil || loginReq.ClientID == "" {
				logEvent(logger, cfg.LogFormat, "warn", "invalid_login_payload", map[string]any{"channel": clientID})
				_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Expected <login>")))
				return
			}

			tok, ok := processAuthorization(httpClient, cfg.AuthBackendURL, remoteAddr, loginReq)
			if !ok {
				logEvent(logger, cfg.LogFormat, "warn", "auth_failed", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "username": loginReq.ClientID})
				_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				_ = writeEPPPayload(client, []byte(buildAuthFailResponse()))
				return
			}

			authenticated = true
			token = tok
			username = loginReq.ClientID
			logEvent(logger, cfg.LogFormat, "info", "auth_success", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "username": username})
			_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			if err = writeEPPPayload(client, []byte(buildLoginResponse(loginReq.ClTRID))); err != nil {
				logEvent(logger, cfg.LogFormat, "error", "write_login_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
				return
			}

		case strings.Contains(xmlBody, "<logout"):
			clTRID := extractClTRID(payload)
			_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			_ = writeEPPPayload(client, []byte(buildLogoutResponse(clTRID)))
			logEvent(logger, cfg.LogFormat, "info", "client_logout", map[string]any{"channel": clientID, "username": username})
			return

		default:
			if strings.TrimSpace(token) == "" {
				_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Missing session token")))
				return
			}

			respBody, callErr := postEPPCommand(httpClient, cfg.CommandBackendURL, token, payload)
			if callErr != nil {
				logEvent(logger, cfg.LogFormat, "error", "backend_command_failed", map[string]any{"channel": clientID, "username": username, "error": callErr.Error()})
				_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Unexpected server error")))
				return
			}
			_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			if err = writeEPPPayload(client, respBody); err != nil {
				logEvent(logger, cfg.LogFormat, "error", "write_command_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
				return
			}
		}
	}
}

func processAuthorization(httpClient *http.Client, authURL, clientIP string, loginReq loginXML) (string, bool) {
	payload, err := json.Marshal(authRequest{
		EppUsername:           loginReq.ClientID,
		EppPassword:           loginReq.Password,
		EppNewPassword:        loginReq.NewPassword,
		ServerCertificateHash: "",
		IPAddress:             clientIP,
	})
	if err != nil {
		return "", false
	}

	req, err := http.NewRequest(http.MethodPost, authURL, bytes.NewReader(payload))
	if err != nil {
		return "", false
	}
	req.Header.Set("authentication", "")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}

	var parsed authResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.ResponseCode, "00") {
		return "", false
	}
	return parsed.EppSessionToken, true
}

func postEPPCommand(httpClient *http.Client, backendURL, token string, payload []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, backendURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authentication", token)
	req.Header.Set("Content-Type", "application/xml")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func parseLoginXML(payload []byte) (loginXML, error) {
	var msg loginXML
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&msg); err != nil {
		return loginXML{}, err
	}
	msg.ClientID = strings.TrimSpace(msg.ClientID)
	msg.Password = strings.TrimSpace(msg.Password)
	msg.NewPassword = strings.TrimSpace(msg.NewPassword)
	msg.ClTRID = strings.TrimSpace(msg.ClTRID)
	return msg, nil
}

func extractClTRID(payload []byte) string {
	msg, err := parseLoginXML(payload)
	if err != nil {
		return ""
	}
	return msg.ClTRID
}

func newRateLimiter(cfg Config) *rateLimiter {
	maxKeys := cfg.RateLimitMaxKeys
	if maxKeys <= 0 {
		maxKeys = 100000
	}
	return &rateLimiter{buckets: make(map[string]map[string][]bucket), maxKeys: maxKeys}
}

func (r *rateLimiter) Allow(ip, username, channelID, commandType string, cfg Config) bool {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.allowForScope("ip", ip, cfg.IPRateLimitRules, now) {
		return false
	}
	if username != "" && !r.allowForScope("client", username, cfg.ClientRateLimit, now) {
		return false
	}
	if !r.allowForScope("channel", channelID, cfg.ChannelRateLimit, now) {
		return false
	}

	switch commandType {
	case "write":
		if !r.allowForScope("write", scopedKey("ip", ip), cfg.WriteIPRateLimit, now) {
			return false
		}
		if username != "" && !r.allowForScope("write", scopedKey("client", username), cfg.WriteClientLimit, now) {
			return false
		}
		if !r.allowForScope("write-legacy", scopedKey("ip", fallbackKey(username, ip)), cfg.WriteRateLimit, now) {
			return false
		}
		return true
	case "read":
		if !r.allowForScope("read", scopedKey("ip", ip), cfg.ReadIPRateLimit, now) {
			return false
		}
		if username != "" && !r.allowForScope("read", scopedKey("client", username), cfg.ReadClientLimit, now) {
			return false
		}
		if !r.allowForScope("read-legacy", scopedKey("ip", fallbackKey(username, ip)), cfg.ReadRateLimit, now) {
			return false
		}
		return true
	default:
		return true
	}
}

func classifyCommandType(payload []byte) string {
	xmlBody := strings.ToLower(string(payload))

	if strings.Contains(xmlBody, "<login") || strings.Contains(xmlBody, "<logout") {
		return "read"
	}

	objects := []string{"domain", "host", "contact"}
	writeVerbs := []string{"create", "update", "renew", "delete", "transfer"}
	for _, obj := range objects {
		for _, verb := range writeVerbs {
			if strings.Contains(xmlBody, "<"+obj+":"+verb) {
				return "write"
			}
		}
		if strings.Contains(xmlBody, "<"+obj+":check") || strings.Contains(xmlBody, "<"+obj+":info") {
			return "read"
		}
	}

	if strings.Contains(xmlBody, ":check") || strings.Contains(xmlBody, ":info") || strings.Contains(xmlBody, "<poll") || strings.Contains(xmlBody, ":poll") {
		return "read"
	}

	return ""
}

func fallbackKey(username, ip string) string {
	if username != "" {
		return username
	}
	return ip
}

func scopedKey(scope, key string) string {
	return scope + ":" + key
}

func (r *rateLimiter) allowForScope(scope, key string, rules []rateLimitRule, now time.Time) bool {
	if len(rules) == 0 {
		return true
	}
	scopeBuckets, ok := r.buckets[scope]
	if !ok {
		scopeBuckets = map[string][]bucket{}
		r.buckets[scope] = scopeBuckets
	}
	if _, exists := scopeBuckets[key]; !exists && len(scopeBuckets) >= r.maxKeys {
		return false
	}

	buckets := scopeBuckets[key]
	if len(buckets) != len(rules) {
		buckets = make([]bucket, len(rules))
	}

	for idx, rule := range rules {
		if rule.limit <= 0 {
			continue
		}
		b := buckets[idx]
		if b.windowStart.IsZero() || now.Sub(b.windowStart) >= rule.window {
			buckets[idx] = bucket{windowStart: now, count: 1}
			continue
		}
		if b.count >= rule.limit {
			scopeBuckets[key] = buckets
			return false
		}
		b.count++
		buckets[idx] = b
	}

	scopeBuckets[key] = buckets
	return true
}

func logEvent(logger *log.Logger, format, level, event string, fields map[string]any) {
	if strings.EqualFold(format, "text") {
		logger.Printf("level=%s event=%s fields=%v", level, event, fields)
		return
	}
	payload := map[string]any{"level": level, "event": event}
	for k, v := range fields {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Printf("level=error event=log_marshal_failed err=%v", err)
		return
	}
	logger.Println(string(raw))
}

func readEPPPayload(reader *bufio.Reader, maxFrameSize int) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}

	totalLen := binary.BigEndian.Uint32(header)
	if totalLen < 5 {
		return nil, fmt.Errorf("invalid epp frame length: %d", totalLen)
	}
	if maxFrameSize > 0 && totalLen > uint32(maxFrameSize) {
		return nil, fmt.Errorf("frame too large: %d > %d", totalLen, maxFrameSize)
	}

	payload := make([]byte, totalLen-4)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeEPPPayload(dst net.Conn, payload []byte) error {
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(frame)))
	copy(frame[4:], payload)
	_, err := dst.Write(frame)
	return err
}

func extractEPPUsername(payload []byte) string {
	msg, err := parseLoginXML(payload)
	if err != nil {
		return ""
	}
	return msg.ClientID
}

func buildRateLimitExceededResponse() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2400"><msg>Rate limit exceeded; please retry later</msg></result><trID><svTRID>rate-limit</svTRID></trID></response></epp>`)
}

func buildGreetingResponse() string {
	svDate := time.Now().UTC().Format(time.RFC3339)
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><greeting><svID>epp.adg.id</svID><svDate>` + svDate + `</svDate><svcMenu><version>1.0</version><lang>en</lang><objURI>urn:ietf:params:xml:ns:domain-1.0</objURI><objURI>urn:ietf:params:xml:ns:contact-1.0</objURI><objURI>urn:ietf:params:xml:ns:host-1.0</objURI><svcExtension><extURI>urn:ietf:params:xml:ns:secDNS-1.1</extURI><extURI>urn:ietf:params:xml:ns:launch-1.0</extURI><extURI>urn:ietf:params:xml:ns:rgp-1.0</extURI></svcExtension></svcMenu><dcp><access><all/></access><statement><purpose><admin/><prov/></purpose><recipient><ours/><public/></recipient><retention><stated/></retention></statement></dcp></greeting></epp>`
}

func buildLoginResponse(clTRID string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="1000"><msg>Command completed successfully</msg></result><trID><clTRID>` + clTRID + `</clTRID><svTRID>go-proxy</svTRID></trID></response></epp>`
}

func buildAuthFailResponse() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2200"><msg>Authentication failed</msg></result><trID><svTRID>go-proxy</svTRID></trID></response></epp>`
}

func buildErrorResponse(message string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2004"><msg>` + message + `</msg></result></response></epp>`
}

func buildLogoutResponse(clTRID string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="1500"><msg>Command completed successfully; ending session</msg></result><trID><clTRID>` + clTRID + `</clTRID><svTRID>go-proxy</svTRID></trID></response></epp>`
}

func remoteIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrFirst(keys []string, fallback string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return fallback
}

func discoverEnvFile() string {
	quick := flag.NewFlagSet("quick", flag.ContinueOnError)
	quick.SetOutput(io.Discard)
	pathFlag := quick.String("env-file", envOr("EPP_ENV_FILE", ".env"), "env file")
	_ = quick.Parse(os.Args[1:])
	return *pathFlag
}

func loadDotEnv(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sep := strings.Index(line, "=")
		if sep <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		value := strings.TrimSpace(line[sep+1:])
		value = strings.Trim(value, "\"'")
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func resolveAddr(portOrAddr string) string {
	v := strings.TrimSpace(portOrAddr)
	if v == "" {
		return ":700"
	}
	if strings.Contains(v, ":") {
		return v
	}
	return ":" + v
}

func durationWithFallback(value string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if sec, err := strconv.Atoi(v); err == nil {
		return time.Duration(sec) * time.Second
	}
	return fallback
}

func boolWithFallback(value string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return fallback
	}
}

func intWithFallback(value string, fallback int) int {
	i, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return i
}

func parseRateLimitRules(raw string) []rateLimitRule {
	parts := strings.Split(raw, ",")
	rules := make([]rateLimitRule, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lr := strings.SplitN(p, "/", 2)
		if len(lr) != 2 {
			continue
		}
		limit, err := strconv.Atoi(strings.TrimSpace(lr[0]))
		if err != nil || limit <= 0 {
			continue
		}
		window, ok := parseRateLimitWindow(strings.TrimSpace(lr[1]))
		if !ok {
			continue
		}
		rules = append(rules, rateLimitRule{limit: limit, window: window})
	}
	return rules
}

func parseRateLimitWindow(raw string) (time.Duration, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d, true
	}
	units := map[string]time.Duration{
		"second":  time.Second,
		"seconds": time.Second,
		"minute":  time.Minute,
		"minutes": time.Minute,
		"hour":    time.Hour,
		"hours":   time.Hour,
	}
	if d, ok := units[v]; ok {
		return d, true
	}
	return 0, false
}

func parseTLSClientAuth(mode string) tls.ClientAuthType {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "NONE":
		return tls.NoClientCert
	case "OPTIONAL":
		return tls.VerifyClientCertIfGiven
	default:
		return tls.RequireAndVerifyClientCert
	}
}

func init() {
	flag.String("env-file", envOr("EPP_ENV_FILE", ".env"), "path to .env configuration")
}

func boolFromEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "1" || v == "true" || v == "TRUE" || v == "yes" || v == "YES" {
		return true
	}
	if v == "0" || v == "false" || v == "FALSE" || v == "no" || v == "NO" {
		return false
	}
	return fallback
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func intFromEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}
