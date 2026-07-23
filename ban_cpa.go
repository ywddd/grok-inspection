package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// errAuthFileNotFound means the credential no longer exists in CPA.
// Unban should still clear the local ban record in this case.
var errAuthFileNotFound = errors.New("auth file not found")

func disableAuthInCPA(authID string) error {
	return setAuthDisabledInCPA(authID, true, cpaManagementPasswordOrCached())
}

func enableAuthInCPA(authID string, password string) error {
	return enableAuthInCPAWithOrigin(authID, password, nil)
}

// enableAuthInCPAWithOrigin re-enables an account, optionally carrying a
// detached Origin-only header map for management transport fallback.
func enableAuthInCPAWithOrigin(authID string, password string, originHeaders http.Header) error {
	return setAuthDisabledInCPAWithOrigin(authID, false, password, originHeaders)
}

// enableAuthInCPAAllowMissing re-enables an account. If the auth file is already
// gone from CPA, it returns enabled=false with a nil error so callers can drop
// the local ban record.
func enableAuthInCPAAllowMissing(authID string, password string) (enabled bool, err error) {
	return enableAuthInCPAAllowMissingWithOrigin(authID, password, nil)
}

// enableAuthInCPAAllowMissingWithOrigin is the Origin-aware enable used by
// operator unban paths. Background auto-restore keeps the nil-header wrapper.
func enableAuthInCPAAllowMissingWithOrigin(authID string, password string, originHeaders http.Header) (enabled bool, err error) {
	err = enableAuthInCPAWithOrigin(authID, password, originHeaders)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errAuthFileNotFound) {
		return false, nil
	}
	return false, err
}

func isAuthFileNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errAuthFileNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "auth file not found") ||
		strings.Contains(msg, "file not found") ||
		(strings.Contains(msg, "http 404") && strings.Contains(msg, "not found"))
}

func setAuthDisabledInCPA(authID string, disabled bool, password string) error {
	return setAuthDisabledInCPAWithOrigin(authID, disabled, password, nil)
}

// setAuthDisabledInCPAWithOrigin patches auth disabled state. originHeaders must
// be Origin-only (or nil); secrets are never read from that map for the Bearer
// token (password is passed separately).
func setAuthDisabledInCPAWithOrigin(authID string, disabled bool, password string, originHeaders http.Header) error {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return fmt.Errorf("auth_id is required")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		password = cpaManagementPasswordOrCached()
	}
	if password == "" {
		return fmt.Errorf("CPA management password is unavailable")
	}

	body, errMarshal := json.Marshal(map[string]any{
		"name":     authID,
		"disabled": disabled,
	})
	if errMarshal != nil {
		return errMarshal
	}
	_, raw, err := callCPAManagementWithAuth(http.MethodPatch, "/v0/management/auth-files/status", body, password, originHeaders)
	if err != nil {
		if isAuthFileNotFoundError(err) || isAuthFileNotFoundResponse(0, raw) {
			return fmt.Errorf("%w: %s", errAuthFileNotFound, strings.TrimSpace(string(raw)))
		}
		return err
	}
	return nil
}

func isAuthFileNotFoundResponse(statusCode int, raw []byte) bool {
	if statusCode == http.StatusNotFound {
		return true
	}
	body := strings.ToLower(strings.TrimSpace(string(raw)))
	if body == "" {
		return false
	}
	return strings.Contains(body, "auth file not found") ||
		strings.Contains(body, `"error":"auth file not found"`) ||
		strings.Contains(body, "file not found")
}
