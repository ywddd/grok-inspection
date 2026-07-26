package main

// Host config discovery for issues #24/#25: the plugin runs in-process with
// CPA, so CPA's own config.yaml (CWD or -config flag) is the authoritative
// source for the real management listen port and TLS state. Request Host or
// Origin ports can be external Docker/panel-mapped ports that CPA never
// listens on inside the container.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type hostListenConfig struct {
	port string
	tls  bool
}

var hostListenCache = struct {
	mu     sync.Mutex
	loaded bool
	cfg    hostListenConfig
	ok     bool
}{}

// hostConfigListen reads CPA's config.yaml once and caches the result; port
// changes require a CPA restart, which also reloads the plugin.
func hostConfigListen() (hostListenConfig, bool) {
	hostListenCache.mu.Lock()
	defer hostListenCache.mu.Unlock()
	if !hostListenCache.loaded {
		hostListenCache.cfg, hostListenCache.ok = readHostListenConfig(hostConfigPath(os.Args[1:]))
		hostListenCache.loaded = true
	}
	return hostListenCache.cfg, hostListenCache.ok
}

// hostConfigPath mirrors CPA's own bootstrap lookup: -config/--config flag
// first, else config.yaml in the process working directory.
func hostConfigPath(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return defaultHostConfigPath()
		case arg == "-config" || arg == "--config":
			if i+1 < len(args) {
				return args[i+1]
			}
			return defaultHostConfigPath()
		case strings.HasPrefix(arg, "-config="):
			return strings.TrimPrefix(arg, "-config=")
		case strings.HasPrefix(arg, "--config="):
			return strings.TrimPrefix(arg, "--config=")
		}
	}
	return defaultHostConfigPath()
}

func defaultHostConfigPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(wd, "config.yaml")
}

func readHostListenConfig(path string) (hostListenConfig, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return hostListenConfig{}, false
	}
	return parseHostListenConfig(raw), true
}

// parseHostListenConfig extracts top-level "port" and the "enable" flag inside
// the top-level "tls" block with line-level parsing (no yaml dependency,
// matching decodeConfig style). Indented port keys in other blocks are ignored.
func parseHostListenConfig(raw []byte) hostListenConfig {
	out := hostListenConfig{}
	inTLS := false
	tlsSeen := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := line[0] == ' ' || line[0] == '\t'
		if !indented {
			inTLS = false
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if idx := strings.Index(value, "#"); idx >= 0 {
			value = value[:idx]
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if !indented {
			switch key {
			case "port":
				out.port = validTCPPort(value)
			case "tls":
				inTLS = true
			}
			continue
		}
		if inTLS && !tlsSeen && key == "enable" {
			if parsed, err := strconv.ParseBool(value); err == nil {
				out.tls = parsed
				tlsSeen = true
			}
		}
	}
	return out
}
