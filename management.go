package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	cpaManagementBaseURL = "http://127.0.0.1:8317"
	cpaManagementClient  = &http.Client{Timeout: 8 * time.Second}
	cpaManagementDo      = func(req *http.Request) (*http.Response, error) {
		return cpaManagementClient.Do(req)
	}
)

func cpaManagementPassword() string {
	return firstNonEmpty(os.Getenv("MANAGEMENT_PASSWORD"), os.Getenv("CPA_MANAGEMENT_KEY"))
}

func extractBearerToken(headers http.Header) string {
	if headers == nil {
		return ""
	}
	// http.Header.Get is case-insensitive for canonical keys.
	auth := strings.TrimSpace(headers.Get("Authorization"))
	if auth == "" {
		// JSON-decoded headers from the host may preserve non-canonical keys.
		for key, values := range headers {
			if strings.EqualFold(strings.TrimSpace(key), "Authorization") && len(values) > 0 {
				auth = strings.TrimSpace(values[0])
				break
			}
		}
	}
	if auth == "" {
		return ""
	}
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return auth
}

func resolveManagementPassword(headers http.Header) string {
	if headers == nil {
		return strings.TrimSpace(cpaManagementPassword())
	}
	if token := extractBearerToken(headers); token != "" {
		return token
	}
	if token := strings.TrimSpace(headers.Get("X-Management-Key")); token != "" {
		return token
	}
	for key, values := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "X-Management-Key") && len(values) > 0 {
			if token := strings.TrimSpace(values[0]); token != "" {
				return token
			}
		}
	}
	return strings.TrimSpace(cpaManagementPassword())
}

func resolveManagementBaseURL(headers http.Header) string {
	_ = headers
	// Prefer explicit env. Never derive the management port from the browser Host
	// header: external reverse proxies (e.g. :1109) are not the CPA listen port.
	if value := firstNonEmpty(os.Getenv("CPA_BASE_URL"), os.Getenv("CPA_MANAGEMENT_BASE_URL")); value != "" {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	if port := strings.TrimSpace(firstNonEmpty(os.Getenv("PORT"), os.Getenv("CPA_PORT"))); port != "" {
		return "http://127.0.0.1:" + port
	}
	return strings.TrimRight(cpaManagementBaseURL, "/")
}

func callCPAManagement(method, path string, body []byte) (int, []byte, error) {
	return callCPAManagementWithAuth(method, path, body, "", nil)
}

func callCPAManagementWithAuth(method, path string, body []byte, password string, headers http.Header) (int, []byte, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		password = resolveManagementPassword(headers)
	}
	if password == "" {
		return 0, nil, fmt.Errorf("CPA management password is unavailable (set MANAGEMENT_PASSWORD on CPA process)")
	}
	baseURL := resolveManagementBaseURL(headers)
	req, errRequest := http.NewRequest(method, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if errRequest != nil {
		return 0, nil, errRequest
	}
	req.Header.Set("Authorization", "Bearer "+password)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, errDo := cpaManagementDo(req)
	if errDo != nil {
		return 0, nil, errDo
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return resp.StatusCode, nil, errRead
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, fmt.Errorf("CPA management API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, raw, nil
}

// findAuthFromResults resolves an auth identity from the in-memory inspection
// list without calling host.auth.list (which is O(n) over all CPA accounts).
