package main

import (
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
