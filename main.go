package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

const (
	pluginName            = "grok-inspection"
	pluginDisplayName     = "Grok 账号巡检"
	pluginVersion         = "0.1.14"
	resourceContentType   = "text/html; charset=utf-8"
	jsonContentType       = "application/json; charset=utf-8"
	managementRoutePrefix = "/plugins/" + pluginName
)

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	UsagePlugin   bool `json:"usage_plugin"`
	Scheduler     bool `json:"scheduler"`
	ManagementAPI bool `json:"management_api"`
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		if err := configure(request); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodUsageHandle:
		return handlePluginMethod(method, request)
	// MethodSchedulerPick intentionally not handled (unknown_method): we never
	// register or occupy CPA's global scheduler slot.
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginDisplayName,
			Version:          pluginVersion,
			Author:           "ywddd",
			GitHubRepository: "https://github.com/ywddd/grok-inspection",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "autoban_enabled", Type: pluginapi.ConfigFieldTypeBoolean, Description: T(LangZH, "cfg_autoban_enabled")},
				{Name: "fallback_hours", Type: pluginapi.ConfigFieldTypeInteger, Description: T(LangZH, "cfg_fallback_hours")},
				{Name: "persist_state", Type: pluginapi.ConfigFieldTypeBoolean, Description: T(LangZH, "cfg_persist_state")},
				{Name: "state_file", Type: pluginapi.ConfigFieldTypeString, Description: T(LangZH, "cfg_state_file")},
				{Name: "log_matches", Type: pluginapi.ConfigFieldTypeBoolean, Description: T(LangZH, "cfg_log_matches")},
			},
		},
		Capabilities: registrationCapabilities{
			UsagePlugin:   true,
			Scheduler:     false, // never monopolize CPA global scheduler slot
			ManagementAPI: true,
		},
	}
}

func managementRegistration() pluginapi.ManagementRegistrationResponse {
	return pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{Method: http.MethodGet, Path: managementRoutePrefix + "/status", Description: "Get Grok inspection status."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/start", Description: "Start a full, incremental, or classify-scoped Grok inspection job."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/stop", Description: "Stop the current Grok inspection job."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/apply", Description: "Apply recommended disable/enable/delete actions asynchronously."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/action", Description: "Disable, enable, or delete one Grok credential asynchronously."},
			{Method: http.MethodGet, Path: managementRoutePrefix + "/bans", Description: "List Grok accounts banned by free-usage / permission-denied / 401."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/unban", Description: "Unban one Grok account and re-enable it in CPA."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/unban-all", Description: "Unban all Grok accounts tracked by autoban."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/autoban-settings", Description: "Update autoban enabled switch and fallback hours."},
		},
		Resources: []pluginapi.ResourceRoute{
			{
				Path:        "/status",
				Menu:        pluginDisplayName,
				Description: T(LangZH, "menu_desc"),
			},
		},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req pluginapi.ManagementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
	}
	return okEnvelope(dispatchManagement(req))
}

func dispatchManagement(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	switch {
	case method == http.MethodGet && matchesResourcePath(req.Path, "/status"):
		return htmlResponse(http.StatusOK, renderUIPage(pluginName))
	case method == http.MethodGet && matchesManagementPath(req.Path, "/status"):
		// Pure memory snapshot — never blocks on host or management HTTP.
		// light=1 / include_results=0: progress meta only (cheap poll during inspect/apply).
		// lang= rewrites known stored reasons for the UI language without mutating memory.
		return jsonResponse(http.StatusOK, engine.snapshotWithLang(statusWantsResults(req), firstQueryValue(req, "lang")))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/start"):
		var body startRequest
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &body)
		}
		if err := engine.start(body); err != nil {
			status := statusFromError(err, http.StatusConflict)
			return jsonResponse(status, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, engine.snapshot(true))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/stop"):
		engine.stop() // also cancels bulk apply / unban jobs
		return jsonResponse(http.StatusOK, engine.snapshot(false))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/apply"):
		var body applyRequest
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid JSON body", "ok": false})
			}
		}
		// Async: returns immediately so status/action stay responsive and delete
		// can call management HTTP without re-entering the same request lock.
		// Capture page Management Key for background delete/auth API calls.
		password := resolveManagementPassword(req.Headers)
		if err := engine.startApply(body, password, req.Headers); err != nil {
			status := http.StatusConflict
			msg := err.Error()
			if strings.Contains(msg, "force_action") || strings.Contains(msg, "requires") || strings.Contains(msg, "no accounts") {
				status = http.StatusBadRequest
			}
			return jsonResponse(status, map[string]any{"error": msg})
		}
		// Slim ack — full account list is only on GET /status (include_results=1).
		snap := engine.snapshot(false)
		return jsonResponse(http.StatusAccepted, map[string]any{
			"ok":          true,
			"accepted":    true,
			"applying":    snap.Applying,
			"apply_total": snap.ApplyTotal,
			"apply_done":  snap.ApplyDone,
		})
	case method == http.MethodPost && matchesManagementPath(req.Path, "/action"):
		var body actionRequest
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
		}
		password := resolveManagementPassword(req.Headers)
		seq, action, err := engine.startAction(body, password, req.Headers)
		if err != nil {
			status := http.StatusConflict
			if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "busy") {
				status = http.StatusBadRequest
				if strings.Contains(err.Error(), "busy") {
					status = http.StatusConflict
				}
			}
			return jsonResponse(status, map[string]any{"error": err.Error(), "ok": false})
		}
		// 202 = accepted only. Clients must poll light /status for recent_row_actions[seq].
		return jsonResponse(http.StatusAccepted, map[string]any{
			"ok":         true,
			"accepted":   true,
			"action":     action,
			"action_seq": seq,
			"name":       firstNonEmpty(body.Name, body.AuthIndex),
		})
	case method == http.MethodGet && matchesManagementPath(req.Path, "/bans"):
		return jsonResponse(http.StatusOK, banStatus())
	case method == http.MethodPost && matchesManagementPath(req.Path, "/unban"):
		var body struct {
			AuthID string `json:"auth_id"`
		}
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid JSON body", "ok": false})
			}
		}
		authID := strings.TrimSpace(body.AuthID)
		if authID == "" {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "missing_auth_id", "ok": false})
		}
		password := resolveManagementPassword(req.Headers)
		enabled, removed, errUnban := unbanOneAccount(authID, password)
		if errUnban != nil {
			status := http.StatusBadRequest
			msg := errUnban.Error()
			if strings.Contains(msg, "busy") {
				status = http.StatusConflict
			} else if strings.Contains(msg, "persist ban state") {
				status = http.StatusInternalServerError
			}
			return jsonResponse(status, map[string]any{"error": msg, "ok": false, "removed": removed, "enabled": enabled})
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"ok":      true,
			"removed": removed,
			"enabled": enabled,
			"missing": !enabled,
		})
	case method == http.MethodPost && matchesManagementPath(req.Path, "/autoban-settings"):
		body := map[string]any{}
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err == nil {
			} else {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error(), "ok": false})
			}
		}
		var enabled *bool
		var fallbackHours *int
		if raw, ok := body["enabled"]; ok {
			switch v := raw.(type) {
			case bool:
				enabled = &v
			}
		}
		if raw, ok := body["fallback_hours"]; ok {
			switch v := raw.(type) {
			case float64:
				// Reject non-integers like 1.9 so callers do not think 1 was accepted.
				if v != float64(int(v)) {
					return jsonResponse(http.StatusBadRequest, map[string]any{"error": "fallback_hours must be an integer between 1 and 168", "ok": false})
				}
				n := int(v)
				fallbackHours = &n
			case int:
				n := v
				fallbackHours = &n
			case json.Number:
				i64, errN := v.Int64()
				if errN != nil {
					return jsonResponse(http.StatusBadRequest, map[string]any{"error": "fallback_hours must be an integer between 1 and 168", "ok": false})
				}
				n := int(i64)
				fallbackHours = &n
			default:
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "fallback_hours must be an integer between 1 and 168", "ok": false})
			}
		}
		// Accept either "enabled" (UI) or registered config name "autoban_enabled".
		if enabled == nil {
			if raw, ok := body["autoban_enabled"]; ok {
				if v, ok := raw.(bool); ok {
					enabled = &v
				}
			}
		}
		if enabled == nil && fallbackHours == nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "missing_settings", "ok": false})
		}
		cfg, err := updateAutobanSettings(enabled, fallbackHours)
		if err == nil {
			return jsonResponse(http.StatusOK, map[string]any{
				"ok":             true,
				"enabled":        cfg.Enabled,
				"fallback_hours": cfg.FallbackHours,
				"persist_state":  cfg.PersistState,
				"state_file":     cfg.StateFile,
			})
		}
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "fallback_hours") {
			status = http.StatusBadRequest
		}
		return jsonResponse(status, map[string]any{"error": err.Error(), "ok": false})
	case method == http.MethodPost && matchesManagementPath(req.Path, "/unban-all"):
		// Async bulk unban so large ban pools do not block the Management handler.
		password := resolveManagementPassword(req.Headers)
		var body struct {
			Category string   `json:"category"`
			AuthIDs  []string `json:"auth_ids"`
		}
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid JSON body", "ok": false})
			}
		}
		if err := startUnbanJob(body.AuthIDs, body.Category, password); err != nil {
			status := http.StatusConflict
			if strings.Contains(err.Error(), "no accounts") {
				status = http.StatusBadRequest
			}
			return jsonResponse(status, map[string]any{"error": err.Error(), "ok": false})
		}
		return jsonResponse(http.StatusAccepted, map[string]any{
			"ok":       true,
			"accepted": true,
			"unban":    unbanJobStatus(),
		})
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "not found", "path": req.Path, "method": method})
	}
}

// statusWantsResults defaults to full results; light polls pass include_results=0 or light=1.
func statusWantsResults(req pluginapi.ManagementRequest) bool {
	if req.Query == nil {
		return true
	}
	if v := strings.TrimSpace(req.Query.Get("include_results")); v != "" {
		return !(v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
	}
	if v := strings.TrimSpace(req.Query.Get("light")); v != "" {
		return !(v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes"))
	}
	return true
}

func matchesManagementPath(path, suffix string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	// Strip query if a gateway put it on Path.
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return path == managementRoutePrefix+suffix ||
		path == "/v0/management"+managementRoutePrefix+suffix
}

func matchesResourcePath(path, suffix string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return path == "/v0/resource/plugins/"+pluginName+suffix
}

func htmlResponse(statusCode int, body []byte) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: statusCode,
		Headers:    http.Header{"Content-Type": []string{resourceContentType}},
		Body:       body,
	}
}

func jsonResponse(statusCode int, payload any) pluginapi.ManagementResponse {
	raw, _ := json.Marshal(payload)
	return pluginapi.ManagementResponse{
		StatusCode: statusCode,
		Headers:    http.Header{"Content-Type": []string{jsonContentType}},
		Body:       raw,
	}
}

func firstQueryValue(req pluginapi.ManagementRequest, key string) string {
	if req.Query == nil {
		return ""
	}
	vals := req.Query[key]
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}
