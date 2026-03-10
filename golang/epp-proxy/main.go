package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
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
	FrontendTLS       bool
	FrontendCert      string
	FrontendKey       string
	AuthBackendURL    string
	CommandBackendURL string
	RateLimitMax      int
	RateLimitWindow   time.Duration
	RateLimitBy       string
}

type rateLimiter struct {
	max    int
	window time.Duration

	mu      sync.Mutex
	buckets map[string]bucket
}

type bucket struct {
	windowStart time.Time
	count       int
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

	logger.Printf("listening on %s and forwarding auth=%s command=%s", cfg.ListenAddr, cfg.AuthBackendURL, cfg.CommandBackendURL)

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
		go func(client net.Conn) {
			defer wg.Done()
			handleConn(cfg, logger, limiter, client)
		}(conn)
	}

	logger.Println("shutting down listener")
	_ = ln.Close()
	wg.Wait()
}

func loadConfig() Config {
	listen := flag.String("listen", envOr("EPP_LISTEN_ADDR", ":700"), "address to listen on")
	connectTimeout := flag.Duration("connect-timeout", durationFromEnv("EPP_CONNECT_TIMEOUT", 5*time.Second), "backend connect timeout")
	idleTimeout := flag.Duration("idle-timeout", durationFromEnv("EPP_IDLE_TIMEOUT", 10*time.Minute), "idle timeout for client connection")
	frontendTLS := flag.Bool("frontend-tls", boolFromEnv("EPP_FRONTEND_TLS", false), "enable TLS listener")
	frontendCert := flag.String("frontend-cert", envOr("EPP_FRONTEND_CERT", "certs/server.crt"), "TLS cert path")
	frontendKey := flag.String("frontend-key", envOr("EPP_FRONTEND_KEY", "certs/server.key"), "TLS key path")
	authURL := flag.String("auth-url", envOr("EPP_AUTH_URL", "http://localhost:8080/PANDI-REGISTRAR-0.1/authRegistrar/"), "backend auth URL")
	commandURL := flag.String("command-url", envOr("EPP_COMMAND_URL", "http://localhost:8080/PANDI-CORE-0.1/processepp/"), "backend command URL")
	rateLimitMax := flag.Int("rate-limit-max", intFromEnv("EPP_RATE_LIMIT_MAX", 10), "maximum EPP commands per rate-limit window")
	rateLimitWindow := flag.Duration("rate-limit-window", durationFromEnv("EPP_RATE_LIMIT_WINDOW", time.Minute), "rate-limit window duration")
	rateLimitBy := flag.String("rate-limit-by", strings.ToLower(envOr("EPP_RATE_LIMIT_BY", "ip_or_username")), "rate-limit key: ip, username, or ip_or_username")
	flag.Parse()

	return Config{
		ListenAddr:        *listen,
		ConnectTimeout:    *connectTimeout,
		IdleTimeout:       *idleTimeout,
		FrontendTLS:       *frontendTLS,
		FrontendCert:      *frontendCert,
		FrontendKey:       *frontendKey,
		AuthBackendURL:    *authURL,
		CommandBackendURL: *commandURL,
		RateLimitMax:      *rateLimitMax,
		RateLimitWindow:   *rateLimitWindow,
		RateLimitBy:       strings.ToLower(*rateLimitBy),
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

	tlsCfg := &tls.Config{Certificates: []tls.Certificate{certificate}}
	return tls.Listen("tcp", cfg.ListenAddr, tlsCfg)
}

func handleConn(cfg Config, logger *log.Logger, limiter *rateLimiter, client net.Conn) {
	defer client.Close()
	clientID := client.RemoteAddr().String()
	remoteAddr := remoteIP(client.RemoteAddr())
	httpClient := &http.Client{Timeout: cfg.ConnectTimeout}

	logger.Printf("[%s] connected", clientID)
	if err := writeEPPPayload(client, []byte(buildGreetingResponse())); err != nil {
		logger.Printf("[%s] failed to send greeting: %v", clientID, err)
		return
	}

	reader := bufio.NewReader(client)
	authenticated := false
	username := ""
	token := ""

	for {
		_ = client.SetReadDeadline(time.Now().Add(cfg.IdleTimeout))
		payload, err := readEPPPayload(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Printf("[%s] read error: %v", clientID, err)
			}
			return
		}

		if extracted := extractEPPUsername(payload); extracted != "" {
			username = extracted
		}

		if !limiter.Allow(remoteAddr, username, cfg.RateLimitBy) {
			logger.Printf("[%s] rate limit exceeded for key=%s", client.RemoteAddr(), limiter.Key(remoteAddr, username, cfg.RateLimitBy))
			if err = writeEPPPayload(client, buildRateLimitExceededResponse()); err != nil {
				logger.Printf("[%s] write rate-limit response failed: %v", clientID, err)
			}
			return
		}

		xmlBody := string(payload)
		switch {
		case !authenticated:
			loginReq, parseErr := parseLoginXML(payload)
			if parseErr != nil || loginReq.ClientID == "" {
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Expected <login>")))
				return
			}

			tok, ok := processAuthorization(httpClient, cfg.AuthBackendURL, remoteAddr, loginReq)
			if !ok {
				_ = writeEPPPayload(client, []byte(buildAuthFailResponse()))
				return
			}

			authenticated = true
			token = tok
			username = loginReq.ClientID
			if err = writeEPPPayload(client, []byte(buildLoginResponse(loginReq.ClTRID))); err != nil {
				logger.Printf("[%s] write login response failed: %v", clientID, err)
				return
			}

		case strings.Contains(xmlBody, "<logout"):
			clTRID := extractClTRID(payload)
			_ = writeEPPPayload(client, []byte(buildLogoutResponse(clTRID)))
			return

		default:
			if strings.TrimSpace(token) == "" {
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Missing session token")))
				return
			}

			respBody, callErr := postEPPCommand(httpClient, cfg.CommandBackendURL, token, payload)
			if callErr != nil {
				logger.Printf("[%s] backend command error: %v", clientID, callErr)
				_ = writeEPPPayload(client, []byte(buildErrorResponse("Unexpected server error")))
				return
			}
			if err = writeEPPPayload(client, respBody); err != nil {
				logger.Printf("[%s] write command response failed: %v", clientID, err)
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
	return &rateLimiter{max: cfg.RateLimitMax, window: cfg.RateLimitWindow, buckets: make(map[string]bucket)}
}

func (r *rateLimiter) Allow(ip, username, mode string) bool {
	if r.max <= 0 {
		return true
	}

	key := r.Key(ip, username, mode)
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	b := r.buckets[key]
	if b.windowStart.IsZero() || now.Sub(b.windowStart) >= r.window {
		b = bucket{windowStart: now, count: 1}
		r.buckets[key] = b
		return true
	}

	if b.count >= r.max {
		return false
	}

	b.count++
	r.buckets[key] = b
	return true
}

func (r *rateLimiter) Key(ip, username, mode string) string {
	switch strings.ToLower(mode) {
	case "username":
		if username != "" {
			return "user:" + username
		}
		return "ip:" + ip
	case "ip":
		return "ip:" + ip
	default:
		if username != "" {
			return "user:" + username
		}
		return "ip:" + ip
	}
}

func readEPPPayload(reader *bufio.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}

	totalLen := binary.BigEndian.Uint32(header)
	if totalLen < 5 {
		return nil, fmt.Errorf("invalid epp frame length: %d", totalLen)
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
