package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ListenAddr                 string
	BackendTimeout             time.Duration
	BackendDialTimeout         time.Duration
	BackendTLSHandshake        time.Duration
	BackendIdleConnTimeout     time.Duration
	IdleTimeout                time.Duration
	WriteTimeout               time.Duration
	FrontendTLS                bool
	FrontendCert               string
	FrontendKey                string
	FrontendCA                 string
	TLSClientAuth              tls.ClientAuthType
	AuthBackendURL             string
	CommandBackendURL          string
	LogoutBackendURL           string
	IPRateLimitRules           []rateLimitRule
	ClientRateLimit            []rateLimitRule
	ChannelRateLimit           []rateLimitRule
	WriteRateLimit             []rateLimitRule
	ReadRateLimit              []rateLimitRule
	ReadIPRateLimit            []rateLimitRule
	WriteIPRateLimit           []rateLimitRule
	ReadClientLimit            []rateLimitRule
	WriteClientLimit           []rateLimitRule
	MaxFrameSize               int
	MaxConns                   int
	RateLimitMaxKeys           int
	BackendMaxIdleConns        int
	BackendMaxIdleConnsPerHost int
	BackendMaxConnsPerHost     int
	BackendResponseMaxBytes    int64
	LogFormat                  string
	RealtimeStatsFile          string
	RealtimeStatsInterval      time.Duration
	RealtimeStatsWriteTimeout  time.Duration
	DomainReadCacheTTL         time.Duration
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]map[string][]bucket
	maxKeys int
}

type connectionTracker struct {
	mu sync.RWMutex

	activeTotal  int
	activeByIP   map[string]int
	activeByUser map[string]int

	totalRead  int
	totalWrite int

	readByIP  map[string]int
	writeByIP map[string]int

	readByUser  map[string]int
	writeByUser map[string]int

	blockedTotal  int
	blockedByIP   map[string]int
	blockedByUser map[string]int
}

type realtimeStats struct {
	Connections connectionSnapshot `json:"connections"`
	Commands    commandSnapshot    `json:"commands"`
	Blocked     blockedSnapshot    `json:"blocked"`
}

type connectionSnapshot struct {
	Total   int            `json:"total"`
	PerIP   map[string]int `json:"per_ip"`
	PerUser map[string]int `json:"per_username"`
}

type commandSnapshot struct {
	TotalRead    int            `json:"total_read"`
	TotalWrite   int            `json:"total_write"`
	ReadPerIP    map[string]int `json:"read_per_ip"`
	WritePerIP   map[string]int `json:"write_per_ip"`
	ReadPerUser  map[string]int `json:"read_per_username"`
	WritePerUser map[string]int `json:"write_per_username"`
}

type blockedSnapshot struct {
	Total   int            `json:"total"`
	PerIP   map[string]int `json:"per_ip"`
	PerUser map[string]int `json:"per_username"`
}

type bucket struct {
	windowStart time.Time
	count       int
}

type commandCache struct {
	mu       sync.RWMutex
	entries  map[string]cacheEntry
	inflight map[string]chan struct{}
	ttl      time.Duration
}

type cacheEntry struct {
	response  []byte
	expiresAt time.Time
}

type rateLimitRule struct {
	limit  int
	window time.Duration
}

type authRequest struct {
	EppUsername           string `json:"eppUsername,omitempty"`
	EppPassword           string `json:"eppPassword,omitempty"`
	EppNewPassword        string `json:"eppNewPassword,omitempty"`
	ServerCertificateHash string `json:"serverCertificateHash"`
	HashCertificate       string `json:"hashCertificate"`
	ClientCertificate     string `json:"clientCertificate"`
	IPAddress             string `json:"ipAddress"`
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

var domainNamePattern = regexp.MustCompile(`(?is)<domain:name(?:\s+[^>]*)?>\s*(?:<!\[CDATA\[(.*?)\]\]>|([^<]+))\s*</domain:name>`)
var responseClTRIDPattern = regexp.MustCompile(`(?is)(<cltrid(?:\s+[^>]*)?>)(.*?)(</cltrid>)`)

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
	domainCache := newCommandCache(cfg.DomainReadCacheTTL)
	tracker := newConnectionTracker()
	httpClient := newBackendHTTPClient(cfg)
	connSlots := make(chan struct{}, max(1, cfg.MaxConns))

	logEvent(logger, cfg.LogFormat, "info", "service_started", map[string]any{"listen_addr": cfg.ListenAddr, "auth_url": cfg.AuthBackendURL, "command_url": cfg.CommandBackendURL, "max_conns": cfg.MaxConns})
	stopStatsWriter := startRealtimeStatsWriter(ctx, logger, cfg, tracker)
	defer stopStatsWriter()

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
			handleConn(cfg, logger, limiter, domainCache, tracker, httpClient, client)
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
	backendTimeout := flag.Duration("backend-timeout", durationWithFallback(envOr("EPP_BACKEND_TIMEOUT", "15s"), 15*time.Second), "overall backend HTTP timeout")
	backendDialTimeout := flag.Duration("backend-dial-timeout", durationWithFallback(envOrFirst([]string{"EPP_BACKEND_DIAL_TIMEOUT", "EPP_CONNECT_TIMEOUT"}, "5s"), 5*time.Second), "backend dial timeout")
	backendTLSHandshake := flag.Duration("backend-tls-timeout", durationWithFallback(envOr("EPP_BACKEND_TLS_HANDSHAKE_TIMEOUT", "3s"), 3*time.Second), "backend TLS handshake timeout")
	backendIdleConnTimeout := flag.Duration("backend-idle-conn-timeout", durationWithFallback(envOr("EPP_BACKEND_IDLE_CONN_TIMEOUT", "90s"), 90*time.Second), "backend idle keep-alive connection timeout")
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
	backendMaxIdleConns := flag.Int("backend-max-idle-conns", intWithFallback(envOr("EPP_BACKEND_MAX_IDLE_CONNS", "2048"), 2048), "maximum idle backend HTTP connections")
	backendMaxIdleConnsPerHost := flag.Int("backend-max-idle-conns-per-host", intWithFallback(envOr("EPP_BACKEND_MAX_IDLE_CONNS_PER_HOST", "1024"), 1024), "maximum idle backend HTTP connections per host")
	backendMaxConnsPerHost := flag.Int("backend-max-conns-per-host", intWithFallback(envOr("EPP_BACKEND_MAX_CONNS_PER_HOST", "0"), 0), "maximum total backend HTTP connections per host; 0 means unlimited")
	backendResponseMaxBytes := flag.Int64("backend-response-max-bytes", int64(intWithFallback(envOr("EPP_BACKEND_RESPONSE_MAX_BYTES", "1048576"), 1048576)), "maximum response body bytes read from backend")
	logFormat := flag.String("log-format", strings.ToLower(envOr("EPP_LOG_FORMAT", "json")), "log format: json or text")
	realtimeStatsFile := flag.String("realtime-stats-file", envOr("EPP_REALTIME_STATS_FILE", "logs/realtime-stats.json"), "path to realtime stats json file")
	realtimeStatsInterval := flag.Duration("realtime-stats-interval", durationWithFallback(envOr("EPP_REALTIME_STATS_INTERVAL", "5s"), 5*time.Second), "refresh interval for realtime stats json file")
	realtimeStatsWriteTimeout := flag.Duration("realtime-stats-write-timeout", durationWithFallback(envOr("EPP_REALTIME_STATS_WRITE_TIMEOUT", "1s"), time.Second), "max duration for each realtime stats file write before skipping")
	domainReadCacheTTL := flag.Duration("domain-read-cache-ttl", durationWithFallback(envOr("EPP_DOMAIN_READ_CACHE_TTL", "30s"), 30*time.Second), "ttl for cached domain read command responses")
	flag.Parse()

	return Config{
		ListenAddr:                 *listen,
		BackendTimeout:             *backendTimeout,
		BackendDialTimeout:         *backendDialTimeout,
		BackendTLSHandshake:        *backendTLSHandshake,
		BackendIdleConnTimeout:     *backendIdleConnTimeout,
		IdleTimeout:                *idleTimeout,
		WriteTimeout:               *writeTimeout,
		FrontendTLS:                *frontendTLS,
		FrontendCert:               *frontendCert,
		FrontendKey:                *frontendKey,
		FrontendCA:                 *frontendCA,
		TLSClientAuth:              parseTLSClientAuth(*tlsClientAuth),
		AuthBackendURL:             *authURL,
		CommandBackendURL:          *commandURL,
		LogoutBackendURL:           *logoutURL,
		IPRateLimitRules:           parseRateLimitRules(*rateLimitIP),
		ClientRateLimit:            parseRateLimitRules(*rateLimitClient),
		ChannelRateLimit:           parseRateLimitRules(*rateLimitChannel),
		WriteRateLimit:             parseRateLimitRules(*rateLimitWrite),
		ReadRateLimit:              parseRateLimitRules(*rateLimitRead),
		ReadIPRateLimit:            parseRateLimitRules(*rateLimitReadIP),
		WriteIPRateLimit:           parseRateLimitRules(*rateLimitWriteIP),
		ReadClientLimit:            parseRateLimitRules(*rateLimitReadClient),
		WriteClientLimit:           parseRateLimitRules(*rateLimitWriteClient),
		MaxFrameSize:               *maxFrameSize,
		MaxConns:                   *maxConns,
		RateLimitMaxKeys:           *rateLimitMaxKeys,
		BackendMaxIdleConns:        *backendMaxIdleConns,
		BackendMaxIdleConnsPerHost: *backendMaxIdleConnsPerHost,
		BackendMaxConnsPerHost:     *backendMaxConnsPerHost,
		BackendResponseMaxBytes:    *backendResponseMaxBytes,
		LogFormat:                  *logFormat,
		RealtimeStatsFile:          *realtimeStatsFile,
		RealtimeStatsInterval:      *realtimeStatsInterval,
		RealtimeStatsWriteTimeout:  *realtimeStatsWriteTimeout,
		DomainReadCacheTTL:         *domainReadCacheTTL,
	}
}

func startRealtimeStatsWriter(ctx context.Context, logger *log.Logger, cfg Config, tracker *connectionTracker) func() {
	filePath := strings.TrimSpace(cfg.RealtimeStatsFile)
	if filePath == "" {
		return func() {}
	}

	interval := cfg.RealtimeStatsInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	writeTimeout := cfg.RealtimeStatsWriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = time.Second
	}

	writerCtx, cancel := context.WithCancel(ctx)
	inFlight := make(chan struct{}, 1)

	writeOnce := func() {
		stats := getAndResetRealtimeStats(tracker)
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			logEvent(logger, cfg.LogFormat, "warn", "realtime_stats_serialize_failed", map[string]any{"error": err.Error()})
			return
		}
		if err = writeJSONWithTimeout(filePath, data, writeTimeout); err != nil {
			logEvent(logger, cfg.LogFormat, "warn", "realtime_stats_write_skipped", map[string]any{"path": filePath, "error": err.Error()})
		}
	}

	select {
	case inFlight <- struct{}{}:
		go func() {
			defer func() { <-inFlight }()
			writeOnce()
		}()
	default:
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-writerCtx.Done():
				return
			case <-ticker.C:
				select {
				case inFlight <- struct{}{}:
					go func() {
						defer func() { <-inFlight }()
						writeOnce()
					}()
				default:
					logEvent(logger, cfg.LogFormat, "warn", "realtime_stats_write_skipped", map[string]any{"path": filePath, "error": "previous_write_still_running"})
				}
			}
		}
	}()

	return cancel
}

func writeJSONWithTimeout(path string, payload []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			errCh <- err
			return
		}
		errCh <- os.WriteFile(path, append(payload, '\n'), 0o644)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("write_timeout_after_%s", timeout)
	case err := <-errCh:
		return err
	}
}

func newBackendHTTPClient(cfg Config) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.BackendDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          max(1, cfg.BackendMaxIdleConns),
		MaxIdleConnsPerHost:   max(1, cfg.BackendMaxIdleConnsPerHost),
		MaxConnsPerHost:       max(0, cfg.BackendMaxConnsPerHost),
		IdleConnTimeout:       cfg.BackendIdleConnTimeout,
		TLSHandshakeTimeout:   cfg.BackendTLSHandshake,
		ExpectContinueTimeout: time.Second,
	}

	return &http.Client{Timeout: cfg.BackendTimeout, Transport: transport}
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

	if requiresClientCAVerification(cfg.TLSClientAuth) {
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

func requiresClientCAVerification(authType tls.ClientAuthType) bool {
	switch authType {
	case tls.VerifyClientCertIfGiven, tls.RequireAndVerifyClientCert:
		return true
	default:
		return false
	}
}

func handleConn(cfg Config, logger *log.Logger, limiter *rateLimiter, domainCache *commandCache, tracker *connectionTracker, httpClient *http.Client, client net.Conn) {
	defer client.Close()
	clientID := client.RemoteAddr().String()
	remoteAddr := remoteIP(client.RemoteAddr())
	certificateHash := ""
	certificatePEM := ""
	if tlsConn, ok := client.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			logEvent(logger, cfg.LogFormat, "warn", "tls_handshake_failed", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "error": err.Error()})
			return
		}
		certificateHash, _ = resolveRegistrarCertificateHash(tlsConn)
		certificatePEM, _ = resolveRegistrarCertificatePEM(tlsConn)
	}
	tracker.connectionOpened(remoteAddr)
	defer tracker.connectionClosed(remoteAddr)

	logEvent(logger, cfg.LogFormat, "info", "client_connected", map[string]any{"channel": clientID, "remote_ip": remoteAddr})
	if err := writeEPPPayload(client, []byte(buildGreetingResponse())); err != nil {
		logEvent(logger, cfg.LogFormat, "error", "greeting_failed", map[string]any{"channel": clientID, "error": err.Error()})
		return
	}

	reader := bufio.NewReader(client)
	authenticated := false
	username := ""
	attachedUsername := ""
	token := ""
	defer func() {
		if attachedUsername != "" {
			tracker.detachUsername(attachedUsername)
		}
	}()

	for {
		_ = client.SetReadDeadline(time.Now().Add(cfg.IdleTimeout))
		payload, err := readEPPPayload(reader, cfg.MaxFrameSize)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logEvent(logger, cfg.LogFormat, "warn", "read_failed", map[string]any{"channel": clientID, "error": err.Error()})
			}
			return
		}

		commandType := classifyCommandType(payload)
		allowed, blockedScope := limiter.AllowWithReason(remoteAddr, username, clientID, commandType, cfg)
		if !allowed {
			tracker.recordBlocked(remoteAddr, username)
			logEvent(logger, cfg.LogFormat, "warn", "rate_limited", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "username": username, "command_type": commandType})
			if blockedScope != "" {
				logEvent(logger, cfg.LogFormat, "warn", "rate_limit_block_scope", map[string]any{"channel": clientID, "scope": blockedScope, "remote_ip": remoteAddr, "username": username})
			}
			_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			if err = writeEPPPayload(client, buildRateLimitExceededResponse()); err != nil {
				logEvent(logger, cfg.LogFormat, "error", "write_rate_limit_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
			}
			return
		}

		if commandType == "read" || commandType == "write" {
			tracker.recordCommand(remoteAddr, username, commandType)
		}

		xmlBody := strings.ToLower(inspectXMLPayload(payload))
		switch {
		case !authenticated:
			loginReq, parseErr := parseLoginXML(payload)
			if parseErr != nil || loginReq.ClientID == "" {
				logEvent(logger, cfg.LogFormat, "warn", "invalid_login_payload", map[string]any{"channel": clientID})
				_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Expected <login>")))
				return
			}

			if strings.TrimSpace(certificateHash) == "" {
				var hashErr error
				certificateHash, hashErr = resolveRegistrarCertificateHash(client)
				if hashErr != nil {
					logEvent(logger, cfg.LogFormat, "warn", "client_certificate_hash_unavailable", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "error": hashErr.Error()})
				}
			}

			if strings.TrimSpace(certificatePEM) == "" {
				var certErr error
				certificatePEM, certErr = resolveRegistrarCertificatePEM(client)
				if certErr != nil {
					logEvent(logger, cfg.LogFormat, "warn", "client_certificate_unavailable", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "error": certErr.Error()})
				}
			}

			tok, ok := processAuthorization(httpClient, cfg.AuthBackendURL, remoteAddr, loginReq, certificateHash, certificatePEM, cfg.BackendResponseMaxBytes)
			if !ok {
				logEvent(logger, cfg.LogFormat, "warn", "auth_failed", map[string]any{"channel": clientID, "remote_ip": remoteAddr, "username": loginReq.ClientID})
				_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				_ = writeEPPPayload(client, []byte(buildAuthFailResponse()))
				return
			}

			authenticated = true
			token = tok
			username = loginReq.ClientID
			if attachedUsername == "" {
				tracker.attachUsername(username)
				attachedUsername = username
			}
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

			cacheKey, cacheable := buildDomainReadCacheKey(payload)
			if cacheable {
				cachedBody, hit, waitCh, reserved := domainCache.GetOrReserve(cacheKey)
				if hit {
					cachedBody = withClientTransactionID(cachedBody, payload)
					_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
					if err = writeEPPPayload(client, cachedBody); err != nil {
						logEvent(logger, cfg.LogFormat, "error", "write_cached_command_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
						return
					}
					logEvent(logger, cfg.LogFormat, "info", "domain_read_cache_hit", map[string]any{"channel": clientID, "username": username, "cache_key": cacheKey})
					continue
				}

				if waitCh != nil {
					<-waitCh
					if cachedAfterWait, ok := domainCache.Get(cacheKey); ok {
						cachedAfterWait = withClientTransactionID(cachedAfterWait, payload)
						_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
						if err = writeEPPPayload(client, cachedAfterWait); err != nil {
							logEvent(logger, cfg.LogFormat, "error", "write_cached_command_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
							return
						}
						logEvent(logger, cfg.LogFormat, "info", "domain_read_cache_hit_after_wait", map[string]any{"channel": clientID, "username": username, "cache_key": cacheKey})
						continue
					}
					_, _, _, reserved = domainCache.GetOrReserve(cacheKey)
				}

				if reserved {
					respBody, callErr := postEPPCommand(httpClient, cfg.CommandBackendURL, token, payload, cfg.BackendResponseMaxBytes)
					if callErr != nil {
						domainCache.CompleteReservation(cacheKey, nil)
						logEvent(logger, cfg.LogFormat, "error", "backend_command_failed", map[string]any{"channel": clientID, "username": username, "error": callErr.Error()})
						_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
						_ = writeEPPPayload(client, []byte(buildErrorResponse("Unexpected server error")))
						return
					}
					domainCache.CompleteReservation(cacheKey, respBody)
					_ = client.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
					if err = writeEPPPayload(client, respBody); err != nil {
						logEvent(logger, cfg.LogFormat, "error", "write_command_response_failed", map[string]any{"channel": clientID, "error": err.Error()})
						return
					}
					continue
				}
			}

			respBody, callErr := postEPPCommand(httpClient, cfg.CommandBackendURL, token, payload, cfg.BackendResponseMaxBytes)
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
			if cacheable {
				domainCache.Set(cacheKey, respBody)
			}
		}
	}
}

func newCommandCache(ttl time.Duration) *commandCache {
	return &commandCache{entries: make(map[string]cacheEntry), inflight: make(map[string]chan struct{}), ttl: ttl}
}

func (c *commandCache) Get(key string) ([]byte, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	if strings.TrimSpace(key) == "" {
		return nil, false
	}
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return append([]byte(nil), entry.response...), true
}

func (c *commandCache) Set(key string, response []byte) {
	if c.ttl <= 0 || strings.TrimSpace(key) == "" || len(response) == 0 {
		return
	}
	c.mu.Lock()
	c.entries[key] = cacheEntry{response: append([]byte(nil), response...), expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *commandCache) GetOrReserve(key string) ([]byte, bool, <-chan struct{}, bool) {
	if c.ttl <= 0 || strings.TrimSpace(key) == "" {
		return nil, false, nil, false
	}

	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		} else {
			resp := append([]byte(nil), entry.response...)
			c.mu.Unlock()
			return resp, true, nil, false
		}
	}

	if waiter, exists := c.inflight[key]; exists {
		c.mu.Unlock()
		return nil, false, waiter, false
	}

	waiter := make(chan struct{})
	c.inflight[key] = waiter
	c.mu.Unlock()
	return nil, false, nil, true
}

func (c *commandCache) CompleteReservation(key string, response []byte) {
	if c.ttl <= 0 || strings.TrimSpace(key) == "" {
		return
	}

	c.mu.Lock()
	if waiter, ok := c.inflight[key]; ok {
		delete(c.inflight, key)
		if len(response) > 0 {
			c.entries[key] = cacheEntry{response: append([]byte(nil), response...), expiresAt: time.Now().Add(c.ttl)}
		}
		close(waiter)
	}
	c.mu.Unlock()
}

func buildDomainReadCacheKey(payload []byte) (string, bool) {
	xmlBody := strings.ToLower(inspectXMLPayload(payload))
	if !strings.Contains(xmlBody, "<domain:check") && !strings.Contains(xmlBody, "<domain:info") {
		return "", false
	}

	domainName := extractDomainName([]byte(xmlBody))
	if domainName == "" {
		return "", false
	}

	commandType := "check"
	if strings.Contains(xmlBody, "<domain:info") {
		commandType = "info"
	}

	return commandType + ":" + domainName, true
}

func extractDomainName(payload []byte) string {
	matches := domainNamePattern.FindSubmatch(payload)
	if len(matches) < 3 {
		return ""
	}
	for _, candidate := range matches[1:3] {
		if text := strings.TrimSpace(string(candidate)); text != "" {
			return strings.ToLower(text)
		}
	}
	return ""
}

func newConnectionTracker() *connectionTracker {
	return &connectionTracker{
		activeByIP:    make(map[string]int),
		activeByUser:  make(map[string]int),
		readByIP:      make(map[string]int),
		writeByIP:     make(map[string]int),
		readByUser:    make(map[string]int),
		writeByUser:   make(map[string]int),
		blockedByIP:   make(map[string]int),
		blockedByUser: make(map[string]int),
	}
}

func (t *connectionTracker) connectionOpened(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeTotal++
	t.activeByIP[ip]++
}

func (t *connectionTracker) attachUsername(username string) {
	if strings.TrimSpace(username) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeByUser[username]++
}

func (t *connectionTracker) detachUsername(username string) {
	if strings.TrimSpace(username) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	decrementKey(t.activeByUser, username)
}

func (t *connectionTracker) connectionClosed(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.activeTotal > 0 {
		t.activeTotal--
	}
	decrementKey(t.activeByIP, ip)
}

func (t *connectionTracker) recordCommand(ip, username, commandType string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch commandType {
	case "read":
		t.totalRead++
		t.readByIP[ip]++
		if username != "" {
			t.readByUser[username]++
		}
	case "write":
		t.totalWrite++
		t.writeByIP[ip]++
		if username != "" {
			t.writeByUser[username]++
		}
	}
}

func (t *connectionTracker) recordBlocked(ip, username string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedTotal++
	t.blockedByIP[ip]++
	if username != "" {
		t.blockedByUser[username]++
	}
}

func (t *connectionTracker) snapshot() realtimeStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return realtimeStats{
		Connections: connectionSnapshot{Total: t.activeTotal, PerIP: cloneMap(t.activeByIP), PerUser: cloneMap(t.activeByUser)},
		Commands:    commandSnapshot{TotalRead: t.totalRead, TotalWrite: t.totalWrite, ReadPerIP: cloneMap(t.readByIP), WritePerIP: cloneMap(t.writeByIP), ReadPerUser: cloneMap(t.readByUser), WritePerUser: cloneMap(t.writeByUser)},
		Blocked:     blockedSnapshot{Total: t.blockedTotal, PerIP: cloneMap(t.blockedByIP), PerUser: cloneMap(t.blockedByUser)},
	}
}

func getInternalRealtimeStats(tracker *connectionTracker) realtimeStats {
	if tracker == nil {
		return realtimeStats{}
	}
	return tracker.snapshot()
}

func getAndResetRealtimeStats(tracker *connectionTracker) realtimeStats {
	if tracker == nil {
		return realtimeStats{}
	}
	return tracker.snapshotAndResetCounters()
}

func (t *connectionTracker) snapshotAndResetCounters() realtimeStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := realtimeStats{
		Connections: connectionSnapshot{Total: t.activeTotal, PerIP: cloneMap(t.activeByIP), PerUser: cloneMap(t.activeByUser)},
		Commands:    commandSnapshot{TotalRead: t.totalRead, TotalWrite: t.totalWrite, ReadPerIP: cloneMap(t.readByIP), WritePerIP: cloneMap(t.writeByIP), ReadPerUser: cloneMap(t.readByUser), WritePerUser: cloneMap(t.writeByUser)},
		Blocked:     blockedSnapshot{Total: t.blockedTotal, PerIP: cloneMap(t.blockedByIP), PerUser: cloneMap(t.blockedByUser)},
	}

	t.totalRead = 0
	t.totalWrite = 0
	t.readByIP = make(map[string]int)
	t.writeByIP = make(map[string]int)
	t.readByUser = make(map[string]int)
	t.writeByUser = make(map[string]int)
	t.blockedTotal = 0
	t.blockedByIP = make(map[string]int)
	t.blockedByUser = make(map[string]int)

	return snapshot
}

func cloneMap(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func decrementKey(source map[string]int, key string) {
	if source[key] <= 1 {
		delete(source, key)
		return
	}
	source[key]--
}

func processAuthorization(httpClient *http.Client, authURL, clientIP string, loginReq loginXML, certificateHash, clientCertificate string, maxResponseBytes int64) (string, bool) {
	payload, err := json.Marshal(authRequest{
		EppUsername:           loginReq.ClientID,
		EppPassword:           loginReq.Password,
		EppNewPassword:        loginReq.NewPassword,
		ServerCertificateHash: certificateHash,
		HashCertificate:       certificateHash,
		ClientCertificate:     clientCertificate,
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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", false
	}

	body, err := readBodyWithLimit(resp.Body, maxResponseBytes)
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

func resolveRegistrarCertificateHash(client net.Conn) (string, error) {
	cert, err := resolveRegistrarCertificate(client)
	if err != nil || cert == nil {
		return "", err
	}

	sum := sha1.Sum(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

func resolveRegistrarCertificatePEM(client net.Conn) (string, error) {
	cert, err := resolveRegistrarCertificate(client)
	if err != nil || cert == nil {
		return "", err
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})), nil
}

func resolveRegistrarCertificate(client net.Conn) (*x509.Certificate, error) {
	tlsConn, ok := client.(*tls.Conn)
	if !ok {
		return nil, nil
	}

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no registrar certificate presented by client")
	}

	return state.PeerCertificates[0], nil
}

func postEPPCommand(httpClient *http.Client, backendURL, token string, payload []byte, maxResponseBytes int64) ([]byte, error) {
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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	return readBodyWithLimit(resp.Body, maxResponseBytes)
}

func readBodyWithLimit(body io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	limited := io.LimitReader(body, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("response body exceeded limit of %d bytes", maxBytes)
	}
	return raw, nil
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

func withClientTransactionID(response []byte, requestPayload []byte) []byte {
	clTRID := extractClTRID(requestPayload)
	if clTRID == "" {
		return append([]byte(nil), response...)
	}

	indices := responseClTRIDPattern.FindSubmatchIndex(response)
	if len(indices) == 0 {
		return append([]byte(nil), response...)
	}

	updated := make([]byte, 0, len(response)+len(clTRID))
	updated = append(updated, response[:indices[4]]...)
	updated = append(updated, []byte(escapeXML(clTRID))...)
	updated = append(updated, response[indices[5]:]...)
	return updated
}

func newRateLimiter(cfg Config) *rateLimiter {
	maxKeys := cfg.RateLimitMaxKeys
	if maxKeys <= 0 {
		maxKeys = 100000
	}
	return &rateLimiter{buckets: make(map[string]map[string][]bucket), maxKeys: maxKeys}
}

func (r *rateLimiter) Allow(ip, username, channelID, commandType string, cfg Config) bool {
	allowed, _ := r.AllowWithReason(ip, username, channelID, commandType, cfg)
	return allowed
}

func (r *rateLimiter) AllowWithReason(ip, username, channelID, commandType string, cfg Config) (bool, string) {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.allowForScope("ip", ip, cfg.IPRateLimitRules, now) {
		return false, "ip"
	}
	if username != "" && !r.allowForScope("client", username, cfg.ClientRateLimit, now) {
		return false, "client"
	}
	if !r.allowForScope("channel", channelID, cfg.ChannelRateLimit, now) {
		return false, "channel"
	}

	switch commandType {
	case "write":
		if !r.allowForScope("write", scopedKey("ip", ip), cfg.WriteIPRateLimit, now) {
			return false, "write-ip"
		}
		if username != "" && !r.allowForScope("write", scopedKey("client", username), cfg.WriteClientLimit, now) {
			return false, "write-client"
		}
		if !r.allowForScope("write-legacy", scopedKey("ip", fallbackKey(username, ip)), cfg.WriteRateLimit, now) {
			return false, "write-legacy"
		}
		return true, ""
	case "read":
		if !r.allowForScope("read", scopedKey("ip", ip), cfg.ReadIPRateLimit, now) {
			return false, "read-ip"
		}
		if username != "" && !r.allowForScope("read", scopedKey("client", username), cfg.ReadClientLimit, now) {
			return false, "read-client"
		}
		if !r.allowForScope("read-legacy", scopedKey("ip", fallbackKey(username, ip)), cfg.ReadRateLimit, now) {
			return false, "read-legacy"
		}
		return true, ""
	default:
		return true, ""
	}
}

func classifyCommandType(payload []byte) string {
	xmlBody := strings.ToLower(inspectXMLPayload(payload))

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

func inspectXMLPayload(payload []byte) string {
	return string(payload)
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

func buildRateLimitExceededResponse() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2400"><msg>Rate limit exceeded; please retry later</msg></result><trID><svTRID>rate-limit</svTRID></trID></response></epp>`)
}

func buildGreetingResponse() string {
	svDate := time.Now().UTC().Format(time.RFC3339)
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><greeting><svID>epp.adg.id</svID><svDate>` + svDate + `</svDate><svcMenu><version>1.0</version><lang>en</lang><objURI>urn:ietf:params:xml:ns:domain-1.0</objURI><objURI>urn:ietf:params:xml:ns:contact-1.0</objURI><objURI>urn:ietf:params:xml:ns:host-1.0</objURI><svcExtension><extURI>urn:ietf:params:xml:ns:secDNS-1.1</extURI><extURI>urn:ietf:params:xml:ns:launch-1.0</extURI><extURI>urn:ietf:params:xml:ns:rgp-1.0</extURI></svcExtension></svcMenu><dcp><access><all/></access><statement><purpose><admin/><prov/></purpose><recipient><ours/><public/></recipient><retention><stated/></retention></statement></dcp></greeting></epp>`
}

func buildLoginResponse(clTRID string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="1000"><msg>Command completed successfully</msg></result><trID><clTRID>` + escapeXML(clTRID) + `</clTRID><svTRID>go-proxy</svTRID></trID></response></epp>`
}

func buildAuthFailResponse() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2200"><msg>Authentication failed</msg></result><trID><svTRID>go-proxy</svTRID></trID></response></epp>`
}

func buildErrorResponse(message string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2004"><msg>` + escapeXML(message) + `</msg></result></response></epp>`
}

func buildLogoutResponse(clTRID string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="1500"><msg>Command completed successfully; ending session</msg></result><trID><clTRID>` + escapeXML(clTRID) + `</clTRID><svTRID>go-proxy</svTRID></trID></response></epp>`
}

func escapeXML(value string) string {
	buf := &bytes.Buffer{}
	if err := xml.EscapeText(buf, []byte(value)); err != nil {
		return ""
	}
	return buf.String()
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
		return tls.RequestClientCert
	default:
		return tls.RequireAnyClientCert
	}
}

func init() {
	flag.String("env-file", envOr("EPP_ENV_FILE", ".env"), "path to .env configuration")
}
