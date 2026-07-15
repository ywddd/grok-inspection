package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type probeError struct {
	Code    string
	Message string
}

type classifyInput struct {
	ChatStatus   int
	ChatCode     string
	ChatError    string
	Disabled     bool
	RequestError string
}

type classifyResult struct {
	Classification string `json:"classification"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
}

func lower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func containsAny(text string, needles ...string) bool {
	value := lower(text)
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		if strings.Contains(value, lower(needle)) {
			return true
		}
	}
	return false
}

// isFreeUsageExhausted reports true only for Grok free-tier exhaustion.
// Real example:
//   {"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage for model ..."}
// Bare HTTP 429 / generic rate-limit text must not match.
func isFreeUsageExhausted(code, message string) bool {
	blob := lower(code) + " " + lower(message)
	// code already contains free-usage-exhausted for subscription:free-usage-exhausted
	return containsAny(blob,
		"free-usage-exhausted",
		"used all the included free usage",
		"included free usage has been exhausted",
	)
}

func isXAIEntry(provider, name, entryType string) bool {
	provider = lower(provider)
	entryType = lower(entryType)
	name = lower(name)
	if provider == "xai" || provider == "x-ai" || provider == "grok" {
		return true
	}
	if entryType == "xai" || entryType == "x-ai" || entryType == "grok" {
		return true
	}
	return strings.HasPrefix(name, "xai-") || strings.HasPrefix(name, "grok-")
}

func isDisabledEntry(disabled bool, status string) bool {
	if disabled {
		return true
	}
	status = lower(status)
	return status == "disabled" || status == "inactive"
}

func shouldInspectEntry(provider, name, entryType string, disabled bool, status string, includeDisabled, onlyDisabled bool) bool {
	if !isXAIEntry(provider, name, entryType) {
		return false
	}
	isDisabled := isDisabledEntry(disabled, status)
	if onlyDisabled {
		return isDisabled
	}
	if !includeDisabled && isDisabled {
		return false
	}
	return true
}

func extractError(body string) probeError {
	body = strings.TrimSpace(body)
	if body == "" {
		return probeError{}
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		// Non-JSON body may still be an error string; keep it short for storage/export.
		return probeError{Message: truncateErrMessage(body, 400)}
	}
	code := asString(data["code"])
	message := ""
	switch errValue := data["error"].(type) {
	case map[string]any:
		if code == "" {
			code = asString(errValue["code"])
		}
		message = firstNonEmpty(asString(errValue["message"]), asString(errValue["error"]))
	case string:
		message = errValue
	}
	if message == "" {
		message = asString(data["message"])
	}
	// Never fall back to the entire response body (healthy /v1/responses payloads are huge
	// and would pollute error_message / bulk export).
	return probeError{Code: code, Message: truncateErrMessage(message, 400)}
}

func truncateErrMessage(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	// Avoid cutting mid-rune.
	r := []rune(value)
	if len(r) <= max {
		return value
	}
	return string(r[:max]) + "…"
}

func classifyProbe(input classifyInput) classifyResult {
	status := input.ChatStatus
	blob := lower(input.ChatCode) + " " + lower(input.ChatError)
	disabled := input.Disabled

	if status == http.StatusUnauthorized || containsAny(blob,
		"token is expired",
		"token has been invalidated",
		"invalid_grant",
		"unauthorized",
	) {
		return classifyResult{Classification: "reauth", Action: "delete", Reason: "认证已过期或失效"}
	}
	// Only Grok free-usage exhaustion (not bare 429 / generic rate limit).
	if isFreeUsageExhausted(input.ChatCode, input.ChatError) {
		action := "disable"
		if disabled {
			action = "keep"
		}
		return classifyResult{Classification: "quota_exhausted", Action: action, Reason: "额度已用尽"}
	}
	// Bare 429 / temporary throttling: do not recommend disable.
	if status == http.StatusTooManyRequests {
		return classifyResult{
			Classification: "probe_error",
			Action:         "keep",
			Reason:         "临时限流 (HTTP 429)，建议稍后重试",
		}
	}
	if status == http.StatusPaymentRequired || status == http.StatusForbidden || containsAny(blob,
		"permission-denied",
		"chat endpoint is denied",
		"deactivated",
		"suspended",
		"banned",
	) {
		action := "disable"
		if disabled {
			action = "keep"
		}
		reason := "对话权限被拒绝"
		if status > 0 {
			reason = fmt.Sprintf("%s (HTTP %d)", reason, status)
		}
		return classifyResult{Classification: "permission_denied", Action: action, Reason: reason}
	}
	if status == http.StatusNotFound || containsAny(blob, "not-found", "does not exist", "no access to it") {
		return classifyResult{Classification: "model_unavailable", Action: "keep", Reason: "测试模型不可用"}
	}
	if status >= 200 && status < 300 {
		action := "keep"
		if disabled {
			action = "enable"
		}
		return classifyResult{Classification: "healthy", Action: action, Reason: "对话测试成功"}
	}
	if strings.TrimSpace(input.RequestError) != "" || status > 0 {
		reason := strings.TrimSpace(input.RequestError)
		if reason == "" {
			reason = "探测失败"
			if status > 0 {
				reason = fmt.Sprintf("%s (HTTP %d)", reason, status)
			}
		}
		return classifyResult{Classification: "probe_error", Action: "keep", Reason: reason}
	}
	return classifyResult{Classification: "unknown", Action: "keep", Reason: "无法可靠分类"}
}

func pickModel(body string) string {
	var data struct {
		Data []struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &data)
	ids := make([]string, 0, len(data.Data))
	for _, item := range data.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(item.Model)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	for _, preferred := range []string{"grok-4.5-build-free", "grok-4.5", "grok-4", "grok-3-mini"} {
		for _, id := range ids {
			if id == preferred {
				return preferred
			}
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return defaultProbeModel
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
