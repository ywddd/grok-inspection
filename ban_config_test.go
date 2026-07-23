package main

import "testing"

func TestDefaultConfig(t *testing.T) {
	got := defaultPluginConfig()
	if got.FallbackHours != 24 {
		t.Fatalf("fallback hours = %d, want 24", got.FallbackHours)
	}
	if !got.PersistState {
		t.Fatal("persist state = false, want true")
	}
	if got.StateFile == "" {
		t.Fatal("state file is empty, want default data/grok-inspection/bans.json")
	}
	if !got.Enabled {
		t.Fatal("enabled = false, want true")
	}
	if !got.LogMatches {
		t.Fatal("log matches = false, want true")
	}
}

func TestDecodeConfig(t *testing.T) {
	got, err := decodeConfig([]byte("fallback_hours: 48\npersist_state: false\nstate_file: data/bans.json\nlog_matches: false\n"))
	if err != nil {
		t.Fatalf("decodeConfig() error = %v", err)
	}
	if got.FallbackHours != 48 || got.PersistState || got.StateFile != "data/bans.json" || got.LogMatches {
		t.Fatalf("config = %#v", got)
	}
}

func TestDecodeConfigInvalidFallbackUsesDefault(t *testing.T) {
	for _, raw := range []string{"fallback_hours: 0\n", "fallback_hours: 169\n"} {
		got, err := decodeConfig([]byte(raw))
		if err != nil {
			t.Fatalf("decodeConfig(%q) error = %v", raw, err)
		}
		if got.FallbackHours != 24 {
			t.Fatalf("decodeConfig(%q) fallback = %d, want 24", raw, got.FallbackHours)
		}
	}
}

// CPA appends a top-level enabled:true for the plugin lifecycle. That must not
// override an explicit autoban_enabled:false written earlier in the same YAML.
func TestDecodeConfigIgnoresCPATopLevelEnabled(t *testing.T) {
	raw := []byte("autoban_enabled: false\nfallback_hours: 24\nenabled: true\n")
	got, err := decodeConfig(raw)
	if err != nil {
		t.Fatalf("decodeConfig() error = %v", err)
	}
	if got.Enabled {
		t.Fatalf("Enabled=true; CPA top-level enabled must not override autoban_enabled:false")
	}

	// Reverse order: trailing enabled:true still ignored after autoban_enabled:false.
	raw2 := []byte("enabled: true\nautoban_enabled: false\n")
	got2, err := decodeConfig(raw2)
	if err != nil {
		t.Fatalf("decodeConfig() error = %v", err)
	}
	if got2.Enabled {
		t.Fatalf("Enabled=true after enabled:true then autoban_enabled:false")
	}

	// Bare enabled without autoban_enabled keeps default (true).
	got3, err := decodeConfig([]byte("enabled: false\nfallback_hours: 24\n"))
	if err != nil {
		t.Fatalf("decodeConfig() error = %v", err)
	}
	if !got3.Enabled {
		t.Fatalf("bare enabled:false must not disable autoban; got Enabled=false")
	}

	// Explicit true still works.
	got4, err := decodeConfig([]byte("autoban_enabled: true\nenabled: false\n"))
	if err != nil {
		t.Fatalf("decodeConfig() error = %v", err)
	}
	if !got4.Enabled {
		t.Fatalf("autoban_enabled:true should remain true")
	}
}

func TestConfigureLoadsLifecycleYAML(t *testing.T) {
	cfg := defaultPluginConfig()
	cfg.PersistState = false
	isolatePluginLifecycle(t, cfg, false)

	err := configure([]byte(`{"schema_version":1,"config_yaml":"ZmFsbGJhY2tfaG91cnM6IDcyCnBlcnNpc3Rfc3RhdGU6IGZhbHNlCg=="}`))
	if err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	got := loadedConfig()
	if got.FallbackHours != 72 || got.PersistState {
		t.Fatalf("loaded config = %#v", got)
	}
}
