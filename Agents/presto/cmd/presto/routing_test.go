package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyDotEnvAndModelRoute(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "model-routing.json")

	if err := os.WriteFile(envPath, []byte("DEEPSEEK_API_KEY=test-key\nDEEPSEEK_ENDPOINT=https://api.deepseek.com\nDEEPSEEK_MODEL=deepseek-v4-flash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{
  "default_route": "legato.presto",
  "routes": {
    "legato.presto": {
      "api_key_env": "DEEPSEEK_API_KEY",
      "base_url_env": "DEEPSEEK_ENDPOINT",
      "model_env": "DEEPSEEK_MODEL"
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("DEEPSEEK_ENDPOINT", "")
	t.Setenv("DEEPSEEK_MODEL", "")
	t.Setenv("PRESTO_API_KEY", "")
	t.Setenv("PRESTO_BASE_URL", "")
	t.Setenv("PRESTO_MODEL", "")
	t.Setenv("PRESTO_ROUTE", "")

	if err := applyDotEnv(envPath); err != nil {
		t.Fatal(err)
	}
	routeName, err := applyModelRoute(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if routeName != "legato.presto" {
		t.Fatalf("route = %q", routeName)
	}
	if got := os.Getenv("PRESTO_API_KEY"); got != "test-key" {
		t.Fatalf("PRESTO_API_KEY = %q", got)
	}
	if got := os.Getenv("PRESTO_BASE_URL"); got != "https://api.deepseek.com" {
		t.Fatalf("PRESTO_BASE_URL = %q", got)
	}
	if got := os.Getenv("PRESTO_MODEL"); got != "deepseek-v4-flash" {
		t.Fatalf("PRESTO_MODEL = %q", got)
	}
}

func TestApplyModelRouteAllowsExplicitPrestoOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "model-routing.json")
	if err := os.WriteFile(configPath, []byte(`{
  "default_route": "legato.presto",
  "routes": {
    "legato.presto": {
      "base_url": "https://api.deepseek.com",
      "model": "deepseek-v4-flash"
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PRESTO_API_KEY", "explicit-key")
	t.Setenv("PRESTO_BASE_URL", "https://explicit.example/v1")
	t.Setenv("PRESTO_MODEL", "explicit-model")
	t.Setenv("PRESTO_ROUTE", "")

	if _, err := applyModelRoute(configPath); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("PRESTO_MODEL"); got != "explicit-model" {
		t.Fatalf("PRESTO_MODEL = %q", got)
	}
}
