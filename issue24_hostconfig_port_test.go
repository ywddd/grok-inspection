package main

// Regression tests for issues #24 and #25: the management dialer must prefer
// the port from CPA's own config.yaml over request Host/Origin-derived ports
// (external Docker/panel mappings) and over the 8317 default for headless
// autoban/schedule calls.

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Issue #24: background autoban dials with nil headers on cold start. With a
// custom port in CPA's config.yaml, the dialer must use it instead of 8317.
func TestResolveManagementBaseURLUsesHostConfigPortForNilHeaders(t *testing.T) {
	withIsolatedManagementEnv(t)
	setHostListenConfigForTest(hostListenConfig{port: "9000"}, true)

	if got := resolveManagementBaseURL(nil); got != "http://127.0.0.1:9000" {
		t.Fatalf("resolveManagementBaseURL(nil) = %q, want config.yaml port 9000", got)
	}
}

// Issue #25: the browser reaches CPA through an external mapped port (e.g.
// Docker -p 18317:8317 or the CPA-Manager-Plus panel). That Host/Origin port
// is not a local listen port; config.yaml's port must win.
func TestResolveManagementBaseURLPrefersHostConfigOverRequestPort(t *testing.T) {
	withIsolatedManagementEnv(t)
	setHostListenConfigForTest(hostListenConfig{port: "8317"}, true)

	headers := http.Header{}
	headers.Set("Host", "192.168.5.209:18317")
	headers.Set("Origin", "http://192.168.5.209:18317")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:8317" {
		t.Fatalf("resolveManagementBaseURL = %q, want config.yaml port 8317 over mapped 18317", got)
	}
}

// Env PORT/CPA_PORT and CPA_MANAGEMENT_BASE_URL stay above config.yaml so
// existing operator overrides keep working.
func TestResolveManagementBaseURLEnvStillBeatsHostConfig(t *testing.T) {
	withIsolatedManagementEnv(t)
	setHostListenConfigForTest(hostListenConfig{port: "9000"}, true)

	t.Setenv("PORT", "7000")
	if got := resolveManagementBaseURL(nil); got != "http://127.0.0.1:7000" {
		t.Fatalf("with PORT env: got %q, want 7000", got)
	}

	t.Setenv("CPA_MANAGEMENT_BASE_URL", "http://127.0.0.1:6000")
	if got := resolveManagementBaseURL(nil); got != "http://127.0.0.1:6000" {
		t.Fatalf("with CPA_MANAGEMENT_BASE_URL env: got %q, want 6000", got)
	}
}

// TLS enabled in config.yaml must switch the loopback dial to https.
func TestResolveManagementBaseURLHostConfigTLS(t *testing.T) {
	withIsolatedManagementEnv(t)
	setHostListenConfigForTest(hostListenConfig{port: "8317", tls: true}, true)

	if got := resolveManagementBaseURL(nil); got != "https://127.0.0.1:8317" {
		t.Fatalf("resolveManagementBaseURL = %q, want https on config TLS", got)
	}
}

// Without config.yaml the old behavior stays: request-derived port, then 8317.
func TestResolveManagementBaseURLFallsBackWithoutHostConfig(t *testing.T) {
	withIsolatedManagementEnv(t)
	setHostListenConfigForTest(hostListenConfig{}, false)

	headers := http.Header{}
	headers.Set("Origin", "http://192.168.1.4:9317")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:9317" {
		t.Fatalf("origin-derived = %q, want 9317", got)
	}
	clearDerivedManagementPortCacheForTest()
	if got := resolveManagementBaseURL(nil); got != "http://127.0.0.1:8317" {
		t.Fatalf("default = %q, want 8317", got)
	}
}

func TestParseHostListenConfig(t *testing.T) {
	raw := []byte("# comment\nhost: \"\"\nport: 9317 # custom\nremote-management:\n  port: 1111\n  secret-key: abc\ntls:\n  enable: true\n  cert: \"a.pem\"\nrequest-retry: 3\n")
	cfg := parseHostListenConfig(raw)
	if cfg.port != "9317" {
		t.Fatalf("port = %q, want 9317 (nested ports must be ignored)", cfg.port)
	}
	if !cfg.tls {
		t.Fatalf("tls = false, want true")
	}

	cfg = parseHostListenConfig([]byte("port: 8317\ntls:\n  enable: false\n"))
	if cfg.port != "8317" || cfg.tls {
		t.Fatalf("got %+v, want port 8317 tls=false", cfg)
	}

	// Invalid port is dropped; later unrelated blocks must not flip TLS.
	cfg = parseHostListenConfig([]byte("port: notaport\nquota-exceeded:\n  enable: true\n"))
	if cfg.port != "" || cfg.tls {
		t.Fatalf("got %+v, want empty", cfg)
	}
}

func TestReadHostListenConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 9555\ntls:\n  enable: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, ok := readHostListenConfig(path)
	if !ok || cfg.port != "9555" || cfg.tls {
		t.Fatalf("got %+v ok=%v, want port 9555", cfg, ok)
	}
	if _, ok := readHostListenConfig(filepath.Join(dir, "missing.yaml")); ok {
		t.Fatalf("missing file must return ok=false")
	}
}

func TestHostConfigPathFlagParsing(t *testing.T) {
	if got := hostConfigPath([]string{"-config", "/etc/cpa/config.yaml"}); got != "/etc/cpa/config.yaml" {
		t.Fatalf("-config value: got %q", got)
	}
	if got := hostConfigPath([]string{"--config=/opt/c.yaml"}); got != "/opt/c.yaml" {
		t.Fatalf("--config=: got %q", got)
	}
	def := defaultHostConfigPath()
	if got := hostConfigPath(nil); got != def {
		t.Fatalf("no args: got %q, want %q", got, def)
	}
	if got := hostConfigPath([]string{"--", "-config", "x.yaml"}); got != def {
		t.Fatalf("after --: got %q, want default", got)
	}
}
