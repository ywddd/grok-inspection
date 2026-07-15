package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	cpaManagementBaseURL = "http://127.0.0.1:8317"
	cpaManagementClient  = newManagementHTTPClient()
	cpaManagementDo      = func(req *http.Request) (*http.Response, error) {
		return cpaManagementClient.Do(req)
	}
)

func newManagementHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			// Avoid corporate/container HTTP_PROXY hijacking loopback management calls.
			Proxy: loopbackAwareProxy,
			// CPA local TLS often uses self-signed certs; management is key-protected.
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, //nolint:gosec // local/self-signed management endpoint
			},
		},
	}
}

func loopbackAwareProxy(req *http.Request) (*url.URL, error) {
	if req != nil && req.URL != nil && isLoopbackHost(req.URL.Hostname()) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" {
		return false
	}
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

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

func managementTLSPreferred() bool {
	return envTruthy(os.Getenv("CPA_TLS")) ||
		envTruthy(os.Getenv("CPA_TLS_ENABLE")) ||
		envTruthy(os.Getenv("TLS_ENABLE")) ||
		envTruthy(os.Getenv("CPA_MANAGEMENT_TLS"))
}

func headerValue(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if value := strings.TrimSpace(headers.Get(name)); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func normalizeHTTPOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + u.Host
}

func requestManagementBaseURL(headers http.Header) string {
	return normalizeHTTPOrigin(headerValue(headers, "Origin"))
}

func configuredManagementBaseURL() (string, bool) {
	if value := firstNonEmpty(os.Getenv("CPA_MANAGEMENT_BASE_URL"), os.Getenv("CPA_BASE_URL")); value != "" {
		return strings.TrimRight(strings.TrimSpace(value), "/"), true
	}
	return "", false
}

func resolveManagementBaseURL(headers http.Header) string {
	_ = headers // Request headers are used only as a transport-failure fallback.
	if value, ok := configuredManagementBaseURL(); ok {
		return value
	}
	scheme := "http"
	if managementTLSPreferred() {
		scheme = "https"
	}
	if port := strings.TrimSpace(firstNonEmpty(os.Getenv("PORT"), os.Getenv("CPA_PORT"))); port != "" {
		port = strings.TrimPrefix(port, ":")
		return scheme + "://127.0.0.1:" + port
	}
	if scheme == "https" {
		return "https://127.0.0.1:8317"
	}
	return strings.TrimRight(cpaManagementBaseURL, "/")
}

type managementTransportError struct {
	err error
}

func (e *managementTransportError) Error() string { return e.err.Error() }
func (e *managementTransportError) Unwrap() error { return e.err }

func isManagementTransportError(err error) bool {
	var transportErr *managementTransportError
	return errors.As(err, &transportErr)
}

func isLikelyPlainHTTPAgainstTLS(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "eof") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "malformed http") ||
		strings.Contains(s, "server gave http response to https") ||
		strings.Contains(s, "first record does not look like") ||
		strings.Contains(s, "tls:") ||
		strings.Contains(s, "http2: ")
}

func shouldRetryManagementWithHTTPS(baseURL string, err error) bool {
	if err == nil {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(baseURL)), "http://") {
		return false
	}
	u, parseErr := url.Parse(baseURL)
	if parseErr != nil || !isLoopbackHost(u.Hostname()) {
		return false
	}
	return isLikelyPlainHTTPAgainstTLS(err)
}

func httpsManagementFallbackURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return "https://" + trimmed[len("http://"):]
	}
	return ""
}

func annotateManagementDialError(baseURL string, err error) error {
	if err == nil {
		return nil
	}
	if !isLikelyPlainHTTPAgainstTLS(err) {
		return err
	}
	return fmt.Errorf("%w (if CPA TLS is enabled, set CPA_MANAGEMENT_BASE_URL=https://127.0.0.1:<port> or CPA_TLS=true)", err)
}

func callCPAManagement(method, path string, body []byte) (int, []byte, error) {
	return callCPAManagementWithAuth(method, path, body, "", nil)
}

func executeManagementRequest(method, baseURL, path string, body []byte, password string) (int, []byte, error) {
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
		return 0, nil, &managementTransportError{err: errDo}
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return resp.StatusCode, nil, &managementTransportError{err: errRead}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, fmt.Errorf("CPA management API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, raw, nil
}

func callCPAManagementWithAuth(method, path string, body []byte, password string, headers http.Header) (int, []byte, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		password = resolveManagementPassword(headers)
	}
	if password == "" {
		return 0, nil, fmt.Errorf("CPA management password is unavailable (set MANAGEMENT_PASSWORD on CPA process)")
	}
	_, explicitlyConfigured := configuredManagementBaseURL()
	baseURL := resolveManagementBaseURL(headers)
	status, raw, err := executeManagementRequest(method, baseURL, path, body, password)
	lastBaseURL := baseURL
	lastErr := err
	if err != nil && shouldRetryManagementWithHTTPS(baseURL, err) {
		if alt := httpsManagementFallbackURL(baseURL); alt != "" {
			status2, raw2, err2 := executeManagementRequest(method, alt, path, body, password)
			if err2 == nil {
				return status2, raw2, nil
			}
			lastBaseURL = alt
			lastErr = fmt.Errorf("%v; HTTPS retry failed: %w (set CPA_MANAGEMENT_BASE_URL=https://127.0.0.1:<port> when CPA TLS is on)", err, err2)
		}
	}
	if lastErr == nil {
		return status, raw, nil
	}
	if !explicitlyConfigured && isManagementTransportError(lastErr) {
		originBaseURL := requestManagementBaseURL(headers)
		if originBaseURL != "" && originBaseURL != baseURL && originBaseURL != lastBaseURL {
			status2, raw2, err2 := executeManagementRequest(method, originBaseURL, path, body, password)
			if err2 == nil {
				return status2, raw2, nil
			}
			return 0, nil, fmt.Errorf("%v; request Origin retry failed: %w", lastErr, err2)
		}
	}
	return 0, nil, annotateManagementDialError(lastBaseURL, lastErr)
}

// findAuthFromResults resolves an auth identity from the in-memory inspection
// list without calling host.auth.list (which is O(n) over all CPA accounts).
