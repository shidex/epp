package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ListenAddr      string
	BackendAddr     string
	ConnectTimeout  time.Duration
	IdleTimeout     time.Duration
	FrontendTLS     bool
	FrontendCert    string
	FrontendKey     string
	BackendTLS      bool
	BackendInsecure bool
	RateLimitMax    int
	RateLimitWindow time.Duration
	RateLimitBy     string
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

	logger.Printf("listening on %s and forwarding to %s", cfg.ListenAddr, cfg.BackendAddr)

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
			handleConn(ctx, cfg, logger, limiter, client)
		}(conn)
	}

	logger.Println("shutting down listener")
	_ = ln.Close()
	wg.Wait()
}

func loadConfig() Config {
	listen := flag.String("listen", envOr("EPP_LISTEN_ADDR", ":700"), "address to listen on")
	backend := flag.String("backend", envOr("EPP_BACKEND_ADDR", "127.0.0.1:1700"), "backend address")
	connectTimeout := flag.Duration("connect-timeout", durationFromEnv("EPP_CONNECT_TIMEOUT", 5*time.Second), "backend connect timeout")
	idleTimeout := flag.Duration("idle-timeout", durationFromEnv("EPP_IDLE_TIMEOUT", 10*time.Minute), "idle timeout for both directions")
	frontendTLS := flag.Bool("frontend-tls", boolFromEnv("EPP_FRONTEND_TLS", false), "enable TLS listener")
	frontendCert := flag.String("frontend-cert", envOr("EPP_FRONTEND_CERT", "certs/server.crt"), "TLS cert path")
	frontendKey := flag.String("frontend-key", envOr("EPP_FRONTEND_KEY", "certs/server.key"), "TLS key path")
	backendTLS := flag.Bool("backend-tls", boolFromEnv("EPP_BACKEND_TLS", false), "connect to backend over TLS")
	backendInsecure := flag.Bool("backend-insecure", boolFromEnv("EPP_BACKEND_INSECURE", false), "skip backend TLS certificate verification")
	rateLimitMax := flag.Int("rate-limit-max", intFromEnv("EPP_RATE_LIMIT_MAX", 10), "maximum EPP commands per rate-limit window")
	rateLimitWindow := flag.Duration("rate-limit-window", durationFromEnv("EPP_RATE_LIMIT_WINDOW", time.Minute), "rate-limit window duration")
	rateLimitBy := flag.String("rate-limit-by", strings.ToLower(envOr("EPP_RATE_LIMIT_BY", "ip_or_username")), "rate-limit key: ip, username, or ip_or_username")
	flag.Parse()

	return Config{
		ListenAddr:      *listen,
		BackendAddr:     *backend,
		ConnectTimeout:  *connectTimeout,
		IdleTimeout:     *idleTimeout,
		FrontendTLS:     *frontendTLS,
		FrontendCert:    *frontendCert,
		FrontendKey:     *frontendKey,
		BackendTLS:      *backendTLS,
		BackendInsecure: *backendInsecure,
		RateLimitMax:    *rateLimitMax,
		RateLimitWindow: *rateLimitWindow,
		RateLimitBy:     strings.ToLower(*rateLimitBy),
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

func handleConn(ctx context.Context, cfg Config, logger *log.Logger, limiter *rateLimiter, client net.Conn) {
	defer client.Close()
	clientID := client.RemoteAddr().String()

	backend, err := dialBackend(ctx, cfg)
	if err != nil {
		logger.Printf("[%s] backend dial failed: %v", clientID, err)
		return
	}
	defer backend.Close()

	logger.Printf("[%s] connected", clientID)

	_ = client.SetDeadline(time.Now().Add(cfg.IdleTimeout))
	_ = backend.SetDeadline(time.Now().Add(cfg.IdleTimeout))

	errCh := make(chan error, 2)
	go proxy(errCh, client, backend)
	go relayClientWithRateLimit(errCh, cfg, logger, limiter, client, backend)

	if err := <-errCh; err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		logger.Printf("[%s] proxy ended with error: %v", clientID, err)
	}
	logger.Printf("[%s] disconnected", clientID)
}

func relayClientWithRateLimit(errCh chan<- error, cfg Config, logger *log.Logger, limiter *rateLimiter, client net.Conn, backend net.Conn) {
	reader := bufio.NewReader(client)
	remoteIP := remoteIP(client.RemoteAddr())
	username := ""

	for {
		_ = client.SetReadDeadline(time.Now().Add(cfg.IdleTimeout))
		payload, err := readEPPPayload(reader)
		if err != nil {
			errCh <- err
			return
		}

		if extracted := extractEPPUsername(payload); extracted != "" {
			username = extracted
		}

		if !limiter.Allow(remoteIP, username, cfg.RateLimitBy) {
			logger.Printf("[%s] rate limit exceeded for key=%s", client.RemoteAddr(), limiter.Key(remoteIP, username, cfg.RateLimitBy))
			if writeErr := writeEPPPayload(client, buildRateLimitExceededResponse()); writeErr != nil {
				errCh <- writeErr
				return
			}
			continue
		}

		_ = backend.SetWriteDeadline(time.Now().Add(cfg.IdleTimeout))
		if writeErr := writeEPPPayload(backend, payload); writeErr != nil {
			errCh <- writeErr
			return
		}
	}
}

func dialBackend(ctx context.Context, cfg Config) (net.Conn, error) {
	dialer := net.Dialer{Timeout: cfg.ConnectTimeout}
	if !cfg.BackendTLS {
		return dialer.DialContext(ctx, "tcp", cfg.BackendAddr)
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: cfg.BackendInsecure}
	return tls.DialWithDialer(&dialer, "tcp", cfg.BackendAddr, tlsCfg)
}

func proxy(errCh chan<- error, dst net.Conn, src net.Conn) {
	_, err := io.Copy(dst, src)
	_ = dst.SetDeadline(time.Now())
	errCh <- err
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
	type login struct {
		ClientID string `xml:"clID"`
	}
	type command struct {
		Login login `xml:"login"`
	}
	type epp struct {
		Command command `xml:"command"`
	}

	var msg epp
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&msg); err != nil {
		return ""
	}
	return strings.TrimSpace(msg.Command.Login.ClientID)
}

func buildRateLimitExceededResponse() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="no"?><epp xmlns="urn:ietf:params:xml:ns:epp-1.0"><response><result code="2502"><msg>Session limit exceeded; EPP limit exceeded</msg></result><trID><svTRID>rate-limit</svTRID></trID></response></epp>`)
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
