package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
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
			handleConn(ctx, cfg, logger, client)
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

func handleConn(ctx context.Context, cfg Config, logger *log.Logger, client net.Conn) {
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
	go proxy(errCh, backend, client)
	go proxy(errCh, client, backend)

	if err := <-errCh; err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		logger.Printf("[%s] proxy ended with error: %v", clientID, err)
	}
	logger.Printf("[%s] disconnected", clientID)
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
