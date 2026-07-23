package main

import (
	"net/http"
	"os"
	"strings"
	"sync"
)

// Plugin-level in-memory Management credential cache.
// Populated from authenticated Management API requests (page-provided key).
// Never written to disk.
var managementCredentialCache = struct {
	mu  sync.RWMutex
	key string
}{}

func clearManagementCredentialCacheForTest() {
	managementCredentialCache.mu.Lock()
	managementCredentialCache.key = ""
	managementCredentialCache.mu.Unlock()
}

func rememberManagementCredential(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	managementCredentialCache.mu.Lock()
	managementCredentialCache.key = key
	managementCredentialCache.mu.Unlock()
}

func cachedManagementCredential() string {
	managementCredentialCache.mu.RLock()
	defer managementCredentialCache.mu.RUnlock()
	return strings.TrimSpace(managementCredentialCache.key)
}

// cpaManagementPasswordOrCached returns request-less credential resolution:
// memory cache > MANAGEMENT_PASSWORD / CPA_MANAGEMENT_KEY env.
func cpaManagementPasswordOrCached() string {
	if key := cachedManagementCredential(); key != "" {
		return key
	}
	return cpaManagementPassword()
}

// resolveManagementPassword prefers: request headers > memory cache > env.
// Successful header extraction is remembered for realtime auto-disable paths
// that cannot see the original Management request.
func resolveManagementPassword(headers http.Header) string {
	if headers != nil {
		if token := extractBearerToken(headers); token != "" {
			rememberManagementCredential(token)
			return token
		}
		if token := strings.TrimSpace(headers.Get("X-Management-Key")); token != "" {
			rememberManagementCredential(token)
			return token
		}
		for key, values := range headers {
			if strings.EqualFold(strings.TrimSpace(key), "X-Management-Key") && len(values) > 0 {
				if token := strings.TrimSpace(values[0]); token != "" {
					rememberManagementCredential(token)
					return token
				}
			}
		}
	}
	if key := cachedManagementCredential(); key != "" {
		return key
	}
	return strings.TrimSpace(cpaManagementPassword())
}

// allowInsecureRemoteManagementTLS is a dangerous opt-in for remote self-signed
// CPA management endpoints. Default is false: non-loopback HTTPS must verify.
// Env names are intentionally explicit.
func allowInsecureRemoteManagementTLS() bool {
	return envTruthy(os.Getenv("GROK_INSPECTION_INSECURE_REMOTE_TLS")) ||
		envTruthy(os.Getenv("CPA_MANAGEMENT_TLS_INSECURE"))
}

func managementTLSSkipVerifyForHost(host string) bool {
	if isLoopbackHost(host) {
		return true
	}
	return allowInsecureRemoteManagementTLS()
}
