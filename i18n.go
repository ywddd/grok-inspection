package main

import (
	"fmt"
	"strings"
)

// Lang is a UI/runtime language code.
// Chinese remains the project default to keep Chinese-speaking operators first-class.
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

// normalizeLang maps free-form language tags to supported languages.
// Unknown / empty values default to Chinese.
func normalizeLang(value string) Lang {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return LangZH
	}
	if strings.HasPrefix(value, "en") {
		return LangEN
	}
	if strings.HasPrefix(value, "zh") {
		return LangZH
	}
	return LangZH
}

// messages holds operator-facing runtime copy.
// Machine classification keys stay stable; only human-readable text is localized.
var messages = map[string]map[Lang]string{
	"auth_expired": {
		LangZH: "认证已过期或失效",
		LangEN: "Authentication expired or invalid",
	},
	"quota_exhausted": {
		LangZH: "额度已用尽",
		LangEN: "Quota exhausted",
	},
	"temp_rate_limited": {
		LangZH: "临时限流 (HTTP 429)，建议稍后重试",
		LangEN: "Temporarily rate-limited (HTTP 429); retry later",
	},
	"permission_denied": {
		LangZH: "对话权限被拒绝",
		LangEN: "Chat permission denied",
	},
	"model_unavailable": {
		LangZH: "测试模型不可用",
		LangEN: "Probe model unavailable",
	},
	"chat_ok": {
		LangZH: "对话测试成功",
		LangEN: "Chat probe succeeded",
	},
	"probe_failed": {
		LangZH: "探测失败",
		LangEN: "Probe failed",
	},
	"unable_classify": {
		LangZH: "无法可靠分类",
		LangEN: "Unable to classify reliably",
	},
	"category_with_incremental": {
		LangZH: "分类巡检不能与增量巡检同时使用",
		LangEN: "Category inspection cannot be combined with incremental inspection",
	},
	"incremental_needs_results": {
		LangZH: "增量巡检需要已有结果，请先完整巡检",
		LangEN: "Incremental inspection requires existing results; run a full inspection first",
	},
	"category_needs_results": {
		LangZH: "分类巡检需要已有结果，请先完整巡检",
		LangEN: "Category inspection requires existing results; run a full inspection first",
	},
	"no_accounts_in_category": {
		LangZH: "当前分类下没有可巡检账号",
		LangEN: "No inspectable accounts in the current category",
	},
	"list_accounts_failed": {
		LangZH: "列出账号失败: %s",
		LangEN: "Failed to list accounts: %s",
	},
	"stopped_before_probe": {
		LangZH: "已停止，未探测",
		LangEN: "Stopped before probing",
	},
	"stopped": {
		LangZH: "已停止",
		LangEN: "Stopped",
	},
	"account_missing": {
		LangZH: "Auth 列表中已不存在该账号",
		LangEN: "Account no longer exists in the Auth list",
	},
	"probe_timeout": {
		LangZH: "探测超时（>%s）",
		LangEN: "Probe timed out (>%s)",
	},
	"missing_auth_index": {
		LangZH: "缺少 auth_index",
		LangEN: "Missing auth_index",
	},
	"fallback_disagreed": {
		LangZH: "；备用接口结果不一致，按主探测结果判定",
		LangEN: "; fallback endpoint disagreed; using primary probe result",
	},
	"http_probe_timeout": {
		LangZH: "HTTP 探测超时（%s）: %s %s",
		LangEN: "HTTP probe timed out (%s): %s %s",
	},
	"list_accounts_timeout": {
		LangZH: "列出账号超时（30s）",
		LangEN: "Listing accounts timed out (30s)",
	},
	"menu_name": {
		LangZH: "Grok 账号巡检",
		LangEN: "Grok Account Inspection",
	},
	"menu_desc": {
		LangZH: "Grok 账号巡检与自动禁用（free-usage / 403 / 401）。",
		LangEN: "Grok account inspection and auto-ban (free-usage / 403 / 401).",
	},
	"workers_range": {
		LangZH: "并发必须是 %d 到 %d 之间的整数",
		LangEN: "workers must be an integer between %d and %d",
	},
	"already_running": {
		LangZH: "巡检已在运行",
		LangEN: "inspection already running",
	},
	"busy_row_action": {
		LangZH: "忙碌：行操作进行中",
		LangEN: "busy: row action in progress",
	},

	"cfg_autoban_enabled": {
		LangZH: "是否启用自动禁用（free-usage / permission-denied / 401）。",
		LangEN: "Enable automatic ban for free-usage / permission-denied / 401.",
	},
	"cfg_fallback_hours": {
		LangZH: "没有准确恢复时间时，free-usage-exhausted 的禁用小时数，默认 24。",
		LangEN: "Ban hours for free-usage-exhausted when no exact restore time is known (default 24).",
	},
	"cfg_persist_state": {
		LangZH: "是否将自动禁用状态保存到 state_file。",
		LangEN: "Persist auto-ban state to state_file.",
	},
	"cfg_state_file": {
		LangZH: "自动禁用状态 JSON 路径；留空时使用 data/grok-inspection/bans.json。",
		LangEN: "Auto-ban state JSON path; empty uses data/grok-inspection/bans.json.",
	},
	"cfg_log_matches": {
		LangZH: "是否记录自动禁用命中日志。",
		LangEN: "Log auto-ban match events.",
	},
	"save_autoban_state_failed": {
		LangZH: "保存自动禁用状态失败: %s",
		LangEN: "Failed to save auto-ban state: %s",
	},
	"busy_unban": {
		LangZH: "忙：解禁进行中",
		LangEN: "busy: unban in progress",
	},
	"busy_inspection": {
		LangZH: "忙：巡检进行中",
		LangEN: "busy: inspection running",
	},
	"busy_apply": {
		LangZH: "忙：批量操作进行中",
		LangEN: "busy: bulk apply in progress",
	},
	"busy_generic": {
		LangZH: "忙：有任务进行中",
		LangEN: "busy",
	},
	"act_disable": {
		LangZH: "禁用",
		LangEN: "disable",
	},
	"act_enable": {
		LangZH: "启用",
		LangEN: "enable",
	},
	"act_delete": {
		LangZH: "删除",
		LangEN: "delete",
	},
	"apply_delete_batch": {
		LangZH: "删除批次 %d-%d/%d",
		LangEN: "delete batch %d-%d/%d",
	},
}

// T returns a localized message. Missing keys fall back to Chinese, then the key.
func T(lang Lang, key string, args ...any) string {
	lang = normalizeLang(string(lang))
	entry := messages[key]
	text := entry[lang]
	if text == "" {
		text = entry[LangZH]
	}
	if text == "" {
		text = key
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// localizeKnownReason rewrites a previously stored reason into the requested language
// when it matches a known catalog value. Unknown free-form text is left unchanged.
func localizeKnownReason(lang Lang, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reason
	}
	lang = normalizeLang(string(lang))

	if out, ok := localizeFormattedReason(lang, reason); ok {
		return out
	}

	for key, entry := range messages {
		// Only map human reason-like keys, not menu/control messages.
		switch key {
		case "menu_name", "menu_desc", "workers_range", "already_running", "busy_row_action",
			"busy_unban", "busy_inspection", "busy_apply", "busy_generic",
			"category_with_incremental", "incremental_needs_results", "category_needs_results",
			"no_accounts_in_category", "list_accounts_failed", "list_accounts_timeout",
			"http_probe_timeout", "probe_timeout",
			"act_disable", "act_enable", "act_delete", "apply_delete_batch",
			"cfg_autoban_enabled", "cfg_fallback_hours", "cfg_persist_state", "cfg_state_file", "cfg_log_matches",
			"save_autoban_state_failed":
			continue
		}
		for _, candidate := range entry {
			if candidate == "" {
				continue
			}
			if reason == candidate {
				return T(lang, key)
			}
			// Handle "Permission denied (HTTP 403)" style suffixes.
			prefix := candidate + " (HTTP "
			if strings.HasPrefix(reason, prefix) && strings.HasSuffix(reason, ")") {
				return T(lang, key) + reason[len(candidate):]
			}
			// Handle fallback disagreement suffix appended to a known reason.
			for _, suf := range messages["fallback_disagreed"] {
				if suf == "" || !strings.HasSuffix(reason, suf) {
					continue
				}
				base := strings.TrimSuffix(reason, suf)
				if base == candidate {
					return T(lang, key) + T(lang, "fallback_disagreed")
				}
				if strings.HasPrefix(base, candidate+" (HTTP ") {
					return T(lang, key) + base[len(candidate):] + T(lang, "fallback_disagreed")
				}
			}
		}
	}
	// Also map whole known control/error strings.
	for key, entry := range messages {
		for _, candidate := range entry {
			if candidate != "" && reason == candidate {
				return T(lang, key)
			}
		}
	}
	return reason
}

// localizeFormattedReason rewrites sprintf-style stored reasons (timeouts, list failures).
func localizeFormattedReason(lang Lang, reason string) (string, bool) {
	// HTTP probe timeout: "HTTP 探测超时（25s）: POST url" / "HTTP probe timed out (25s): POST url"
	for _, src := range []Lang{LangZH, LangEN} {
		tpl := messages["http_probe_timeout"][src]
		if tpl == "" {
			continue
		}
		// Convert fmt template to a simple parser: prefix + (dur) + ": " + method + " " + url
		// ZH uses fullwidth parentheses （ ）; EN uses ().
		if src == LangZH {
			const head = "HTTP 探测超时（"
			const mid = "）: "
			if strings.HasPrefix(reason, head) {
				rest := strings.TrimPrefix(reason, head)
				idx := strings.Index(rest, mid)
				if idx > 0 {
					dur := rest[:idx]
					tail := rest[idx+len(mid):]
					method, url, ok := splitMethodURL(tail)
					if ok {
						return T(lang, "http_probe_timeout", dur, method, url), true
					}
				}
			}
		} else {
			const head = "HTTP probe timed out ("
			const mid = "): "
			if strings.HasPrefix(reason, head) {
				rest := strings.TrimPrefix(reason, head)
				idx := strings.Index(rest, mid)
				if idx > 0 {
					dur := rest[:idx]
					tail := rest[idx+len(mid):]
					method, url, ok := splitMethodURL(tail)
					if ok {
						return T(lang, "http_probe_timeout", dur, method, url), true
					}
				}
			}
		}
	}

	// Account-level probe timeout: "探测超时（>55s）" / "Probe timed out (>55s)"
	for _, src := range []Lang{LangZH, LangEN} {
		if src == LangZH {
			const head = "探测超时（>"
			const tail = "）"
			if strings.HasPrefix(reason, head) && strings.HasSuffix(reason, tail) {
				dur := strings.TrimSuffix(strings.TrimPrefix(reason, head), tail)
				return T(lang, "probe_timeout", dur), true
			}
		} else {
			const head = "Probe timed out (>"
			const tail = ")"
			if strings.HasPrefix(reason, head) && strings.HasSuffix(reason, tail) {
				dur := strings.TrimSuffix(strings.TrimPrefix(reason, head), tail)
				return T(lang, "probe_timeout", dur), true
			}
		}
	}

	// List accounts failed: "列出账号失败: x" / "Failed to list accounts: x"
	for _, src := range []Lang{LangZH, LangEN} {
		// Build prefix by formatting with empty and trimming carefully — use static prefixes.
		var prefix string
		if src == LangZH {
			prefix = "列出账号失败: "
		} else {
			prefix = "Failed to list accounts: "
		}
		if strings.HasPrefix(reason, prefix) {
			detail := strings.TrimPrefix(reason, prefix)
			// Nested known reasons (e.g. list-timeout) must translate fully, not only the outer prefix.
			detail = localizeKnownReason(lang, detail)
			return T(lang, "list_accounts_failed", detail), true
		}
	}

	// list_accounts_timeout exact forms already handled by exact match, but keep explicit.
	for _, src := range []Lang{LangZH, LangEN} {
		if reason == messages["list_accounts_timeout"][src] && messages["list_accounts_timeout"][src] != "" {
			return T(lang, "list_accounts_timeout"), true
		}
	}
	return "", false
}

func splitMethodURL(tail string) (method, url string, ok bool) {
	tail = strings.TrimSpace(tail)
	parts := strings.SplitN(tail, " ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	method = strings.TrimSpace(parts[0])
	url = strings.TrimSpace(parts[1])
	if method == "" || url == "" {
		return "", "", false
	}
	return method, url, true
}

func localizedActionVerb(lang Lang, action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "disable":
		return T(lang, "act_disable")
	case "enable":
		return T(lang, "act_enable")
	case "delete":
		return T(lang, "act_delete")
	default:
		return action
	}
}
