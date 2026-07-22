package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type pluginConfig struct {
	FallbackHours int
	PersistState  bool
	StateFile     string
	LogMatches    bool
	Enabled       bool
}

type configYAML struct {
	FallbackHours int    `yaml:"fallback_hours"`
	PersistState  *bool  `yaml:"persist_state"`
	StateFile     string `yaml:"state_file"`
	LogMatches    *bool  `yaml:"log_matches"`
	Enabled       *bool  `yaml:"autoban_enabled"`
}

type lifecycleRequest struct {
	SchemaVersion uint32 `json:"schema_version"`
	ConfigYAML    []byte `json:"config_yaml"`
}

// runtimeSettings holds UI-persisted overrides (toggle switch etc.).
// Survives CPA restart; applied after host YAML so in-page choices stick.
type runtimeSettings struct {
	AutobanEnabled *bool `json:"autoban_enabled,omitempty"`
	FallbackHours  *int  `json:"fallback_hours,omitempty"`
}

var currentConfig atomic.Value

func init() {
	currentConfig.Store(defaultPluginConfig())
}

func defaultPluginConfig() pluginConfig {
	return pluginConfig{
		FallbackHours: 24,
		PersistState:  true,
		StateFile:     defaultBanStateFile(),
		LogMatches:    true,
		Enabled:       true,
	}
}

func defaultBanStateFile() string {
	if dir := strings.TrimSpace(os.Getenv("GROK_INSPECTION_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "bans.json")
	}
	return filepath.Join("data", "grok-inspection", "bans.json")
}

func defaultRuntimeSettingsFile() string {
	if dir := strings.TrimSpace(os.Getenv("GROK_INSPECTION_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	return filepath.Join("data", "grok-inspection", "settings.json")
}

func runtimeSettingsFile(cfg pluginConfig) string {
	if strings.TrimSpace(cfg.StateFile) != "" {
		return filepath.Join(filepath.Dir(cfg.StateFile), "settings.json")
	}
	return defaultRuntimeSettingsFile()
}

func legacyBanStateCandidates() []string {
	out := []string{
		filepath.Join("data", "grok-autoban", "bans.json"),
		filepath.Join("data", "bans.json"),
		"bans.json",
	}
	if dir := strings.TrimSpace(os.Getenv("GROK_AUTOBAN_DATA_DIR")); dir != "" {
		out = append([]string{filepath.Join(dir, "bans.json")}, out...)
	}
	return out
}

func decodeConfig(raw []byte) (pluginConfig, error) {
	cfg := defaultPluginConfig()
	if len(raw) == 0 {
		return cfg, nil
	}

	decoded := configYAML{}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch key {
		case "fallback_hours":
			decoded.FallbackHours, _ = strconv.Atoi(value)
		case "persist_state":
			if parsed, err := strconv.ParseBool(value); err == nil {
				decoded.PersistState = &parsed
			}
		case "state_file":
			decoded.StateFile = value
		case "log_matches":
			if parsed, err := strconv.ParseBool(value); err == nil {
				decoded.LogMatches = &parsed
			}
		case "autoban_enabled":
			// Do not treat CPA top-level "enabled" as autoban_enabled; CPA appends enabled:true.
			if parsed, err := strconv.ParseBool(value); err == nil {
				decoded.Enabled = &parsed
			}
		}
	}
	if decoded.FallbackHours >= 1 && decoded.FallbackHours <= 168 {
		cfg.FallbackHours = decoded.FallbackHours
	}
	if decoded.PersistState != nil {
		cfg.PersistState = *decoded.PersistState
	}
	if strings.TrimSpace(decoded.StateFile) != "" {
		cfg.StateFile = strings.TrimSpace(decoded.StateFile)
	}
	if decoded.LogMatches != nil {
		cfg.LogMatches = *decoded.LogMatches
	}
	if decoded.Enabled != nil {
		cfg.Enabled = *decoded.Enabled
	}
	return cfg, nil
}

func loadRuntimeSettings(path string) runtimeSettings {
	var out runtimeSettings
	if strings.TrimSpace(path) == "" {
		return out
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func applyRuntimeSettings(cfg pluginConfig) pluginConfig {
	rs := loadRuntimeSettings(runtimeSettingsFile(cfg))
	if rs.AutobanEnabled != nil {
		cfg.Enabled = *rs.AutobanEnabled
	}
	if rs.FallbackHours != nil && *rs.FallbackHours >= 1 && *rs.FallbackHours <= 168 {
		cfg.FallbackHours = *rs.FallbackHours
	}
	return cfg
}

func saveRuntimeSettings(cfg pluginConfig) error {
	path := runtimeSettingsFile(cfg)
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	enabled := cfg.Enabled
	hours := cfg.FallbackHours
	payload := runtimeSettings{
		AutobanEnabled: &enabled,
		FallbackHours:  &hours,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// updateAutobanSettings applies UI toggle / hours and persists for next restart.
func updateAutobanSettings(enabled *bool, fallbackHours *int) (pluginConfig, error) {
	cfg := loadedConfig()
	if enabled != nil {
		cfg.Enabled = *enabled
	}
	if fallbackHours != nil {
		if *fallbackHours < 1 || *fallbackHours > 168 {
			return cfg, fmt.Errorf("fallback_hours must be an integer between 1 and 168")
		}
		cfg.FallbackHours = *fallbackHours
	}
	if err := saveRuntimeSettings(cfg); err != nil {
		return cfg, err
	}
	currentConfig.Store(cfg)
	return cfg, nil
}

func configure(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
	}
	cfg, err := decodeConfig(req.ConfigYAML)
	if err != nil {
		return err
	}
	// UI-saved switch/hours win over host YAML so the page toggle sticks.
	cfg = applyRuntimeSettings(cfg)
	currentConfig.Store(cfg)
	loadBanState(cfg)
	startInspectionScheduleLoop()
	return nil
}

func loadBanState(cfg pluginConfig) {
	startBanRestoreLoop()
	if !(cfg.PersistState && cfg.StateFile != "") {
		return
	}
	now := time.Now()
	targetExists := false
	if _, err := os.Stat(cfg.StateFile); err == nil {
		targetExists = true
	}
	if targetExists {
		// Target file exists (even if empty []): never re-import legacy bans.
		if err := activeStore.Load(cfg.StateFile, now); err != nil {
			slog.Warn("grok-inspection: failed to load ban state", "path", cfg.StateFile, "error", err)
		}
		return
	}
	// First run only: migrate leftover bans from the standalone grok-autoban plugin.
	for _, candidate := range legacyBanStateCandidates() {
		if candidate == "" || candidate == cfg.StateFile {
			continue
		}
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		if err := activeStore.Load(candidate, now); err != nil {
			slog.Warn("grok-inspection: failed to migrate ban state", "path", candidate, "error", err)
			continue
		}
		if activeStore.Count() == 0 {
			continue
		}
		if err := activeStore.Save(cfg.StateFile); err != nil {
			slog.Warn("grok-inspection: failed to save migrated ban state", "path", cfg.StateFile, "error", err)
		} else {
			slog.Info("grok-inspection: migrated ban state", "from", candidate, "to", cfg.StateFile, "count", activeStore.Count())
			// Rename legacy file so it is not re-imported if the new file is deleted.
			_ = os.Rename(candidate, candidate+".migrated")
		}
		return
	}
}

func loadedConfig() pluginConfig {
	if cfg, ok := currentConfig.Load().(pluginConfig); ok {
		return cfg
	}
	return defaultPluginConfig()
}
