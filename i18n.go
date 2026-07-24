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
	"spending_limit": {
		LangZH: "额度或订阅受限",
		LangEN: "Spending or subscription limit",
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
	"sample_with_incremental": {
		LangZH: "抽检不能与增量巡检同时使用",
		LangEN: "Sample inspection cannot be combined with incremental inspection",
	},
	"sample_params_required": {
		LangZH: "抽检请填写数量或比例",
		LangEN: "Sample inspection requires a count or percent",
	},
	"sample_count_invalid": {
		LangZH: "抽检数量无效",
		LangEN: "Invalid sample count",
	},
	"sample_percent_invalid": {
		LangZH: "抽检比例须在 0-100 之间",
		LangEN: "Sample percent must be between 0 and 100",
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
		LangZH: "Grok 账号巡检与自动禁用（free-usage / 402 / 403 / 401）。",
		LangEN: "Grok account inspection and auto-ban (free-usage / 402 / 403 / 401).",
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
		LangZH: "是否启用自动禁用（free-usage / spending-limit / permission-denied / 401）。",
		LangEN: "Enable automatic ban for free-usage / spending-limit / permission-denied / 401.",
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
	"invalid_json": {
		LangZH: "请求 JSON 无效",
		LangEN: "invalid JSON body",
	},
	"force_action_invalid": {
		LangZH: "force_action 必须是 disable、enable 或 delete",
		LangEN: "force_action must be disable, enable, or delete",
	},
	"force_action_requires_targets": {
		LangZH: "force_action 需要提供 auth_indexes 或 classifications",
		LangEN: "force_action requires auth_indexes or classifications",
	},
	"no_accounts_matched": {
		LangZH: "当前选择下没有匹配的账号",
		LangEN: "no accounts matched current selection",
	},
	"no_recommended_actions": {
		LangZH: "没有可执行的建议操作",
		LangEN: "no recommended actions",
	},
	"name_or_auth_required": {
		LangZH: "需要提供 name 或 auth_index",
		LangEN: "name or auth_index required",
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
	"auth_not_found": {
		LangZH: "未找到账号: %s",
		LangEN: "auth not found: %s",
	},
	"auth_file_name_missing_for": {
		LangZH: "账号缺少 auth 文件名: %s",
		LangEN: "auth file name missing for %s",
	},
	"auth_file_name_missing": {
		LangZH: "账号缺少 auth 文件名",
		LangEN: "auth file name missing",
	},
	"mgmt_password_unavailable": {
		LangZH: "管理密码不可用",
		LangEN: "CPA management password is unavailable",
	},
	"ban_conflict_superseded": {
		LangZH: "启用被并发自动禁用抢占，账号已重新禁用",
		LangEN: "enable superseded by concurrent ban; account re-disabled",
	},

	"unsupported_action": {
		LangZH: "不支持的操作 %q",
		LangEN: "unsupported action %q",
	},
	"persist_ban_enabled": {
		LangZH: "已在 CPA 启用但保存禁用状态失败: %s",
		LangEN: "enabled in CPA but failed to persist ban state: %s",
	},
	"persist_ban_deleted_local": {
		LangZH: "本地已删除但保存禁用状态失败: %s",
		LangEN: "deleted locally but failed to persist ban state: %s",
	},
	"persist_ban_deleted_cpa": {
		LangZH: "已在 CPA 删除但保存禁用状态失败: %s",
		LangEN: "deleted in CPA but failed to persist ban state: %s",
	},
	"persist_ban_unbanned": {
		LangZH: "已在 CPA 解禁但保存禁用状态失败: %s",
		LangEN: "unbanned in CPA but failed to persist ban state: %s",
	},
	"persist_ban_state": {
		LangZH: "保存禁用状态: %s",
		LangEN: "persist ban state: %s",
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
	original := reason
	reason = strings.TrimSpace(reason)
	if reason == "" {
		// Preserve pure whitespace / empty unknown diagnostics.
		return original
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
			"category_with_incremental", "sample_with_incremental", "sample_params_required", "sample_count_invalid", "sample_percent_invalid", "incremental_needs_results", "category_needs_results",
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
	// Unknown free-form diagnostics must keep original leading/trailing whitespace.
	return original
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

// localizeKnownActionError rewrites plugin-generated fixed action/status errors
// into the requested language. Unknown CPA/HTTP/network free text is preserved.
func localizeKnownActionError(lang Lang, msg string) string {
	original := msg
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return original
	}
	lang = normalizeLang(string(lang))

	if out, ok := matchKnownActionError(lang, msg); ok {
		return out
	}

	// Batch failures are commonly "account: error". Keep the account prefix
	// and localize only a recognized trailing fixed error.
	if i := strings.Index(msg, ": "); i > 0 {
		left := msg[:i]
		right := strings.TrimSpace(msg[i+2:])
		if looksLikeAccountErrorPrefix(left) {
			if out, ok := matchKnownActionError(lang, right); ok {
				return left + ": " + out
			}
		}
	}

	// Reuse reason catalog for fixed status strings (Stopped / list timeout / ...).
	if out := localizeKnownReason(lang, msg); out != msg {
		return out
	}
	if out, ok := localizeApplyProgressMessage(lang, msg); ok {
		return out
	}
	// Unknown free-form diagnostics must keep original leading/trailing whitespace.
	return original
}

func looksLikeAccountErrorPrefix(left string) bool {
	left = strings.TrimSpace(left)
	if left == "" || strings.Contains(left, " ") {
		return false
	}
	lower := strings.ToLower(left)
	if strings.Contains(left, "://") || strings.HasPrefix(lower, "http") {
		return false
	}
	// Avoid treating "auth not found" style phrases as account names.
	if strings.Contains(lower, "auth") || strings.Contains(lower, "password") ||
		strings.Contains(lower, "persist") || strings.Contains(lower, "unsupported") ||
		strings.Contains(lower, "failed") || strings.Contains(lower, "deleted") ||
		strings.Contains(lower, "enabled") || strings.Contains(lower, "unbanned") ||
		strings.Contains(left, "失败") || strings.Contains(left, "缺少") ||
		strings.Contains(left, "未找到") || strings.Contains(left, "不支持") ||
		strings.Contains(left, "保存") || strings.Contains(left, "删除") ||
		strings.Contains(left, "启用") || strings.Contains(left, "解禁") {
		return false
	}
	return true
}

func matchKnownActionError(lang Lang, msg string) (string, bool) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", false
	}

	// Exact fixed phrases (no args).
	for _, key := range []string{
		"stopped", "list_accounts_timeout", "auth_file_name_missing", "mgmt_password_unavailable", "ban_conflict_superseded",
	} {
		for _, src := range []Lang{LangZH, LangEN} {
			cand := messages[key][src]
			if cand != "" && msg == cand {
				return T(lang, key), true
			}
		}
	}
	// Sentinel conflict errors from enable/unban CAS (raw Error() text).
	for _, raw := range []string{
		errBanSupersededByNewerRevision.Error(),
		"unban_conflict: concurrent ban retained", // legacy string if still in logs/UI
	} {
		if msg == raw {
			return T(lang, "ban_conflict_superseded"), true
		}
	}

	// Long EN password form includes an English parenthetical hint.
	enPw := messages["mgmt_password_unavailable"][LangEN]
	zhPw := messages["mgmt_password_unavailable"][LangZH]
	if enPw != "" && (msg == enPw || strings.HasPrefix(msg, enPw+" ") || strings.HasPrefix(msg, enPw+"(") || strings.HasPrefix(msg, enPw+" (")) {
		return T(lang, "mgmt_password_unavailable"), true
	}
	if zhPw != "" && (msg == zhPw || strings.HasPrefix(msg, zhPw+" ") || strings.HasPrefix(msg, zhPw+"(") || strings.HasPrefix(msg, zhPw+" (")) {
		return T(lang, "mgmt_password_unavailable"), true
	}

	// Prefixed templates with a free-form diagnostic tail that must stay intact.
	for _, key := range []string{
		"auth_not_found",
		"auth_file_name_missing_for",
		"persist_ban_enabled",
		"persist_ban_deleted_local",
		"persist_ban_deleted_cpa",
		"persist_ban_unbanned",
		"persist_ban_state",
		"save_autoban_state_failed",
	} {
		if out, ok := matchPrefixedActionError(lang, msg, key); ok {
			return out, true
		}
	}

	// unsupported action "x"
	if out, ok := matchUnsupportedAction(lang, msg); ok {
		return out, true
	}
	return "", false
}

func matchPrefixedActionError(lang Lang, msg, key string) (string, bool) {
	for _, src := range []Lang{LangZH, LangEN} {
		tpl := messages[key][src]
		if tpl == "" || !strings.Contains(tpl, "%s") {
			continue
		}
		head := tpl[:strings.Index(tpl, "%s")]
		if head == "" || !strings.HasPrefix(msg, head) {
			continue
		}
		detail := strings.TrimPrefix(msg, head)
		return T(lang, key, detail), true
	}
	return "", false
}

func matchUnsupportedAction(lang Lang, msg string) (string, bool) {
	for _, src := range []Lang{LangZH, LangEN} {
		tpl := messages["unsupported_action"][src]
		if tpl == "" || !strings.Contains(tpl, "%q") {
			continue
		}
		head := tpl[:strings.Index(tpl, "%q")]
		if !strings.HasPrefix(msg, head) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(msg, head))
		if len(rest) < 2 || rest[0] != '"' {
			continue
		}
		end := strings.LastIndex(rest, `"`)
		if end <= 0 {
			continue
		}
		action := rest[1:end]
		return T(lang, "unsupported_action", action), true
	}
	return "", false
}

// localizeApplyProgressMessage rewrites stored apply_current progress lines.
func localizeApplyProgressMessage(lang Lang, msg string) (string, bool) {
	var a, b, c int
	if n, err := fmt.Sscanf(msg, "delete batch %d-%d/%d", &a, &b, &c); err == nil && n == 3 {
		return T(lang, "apply_delete_batch", a, b, c), true
	}
	if n, err := fmt.Sscanf(msg, "删除批次 %d-%d/%d", &a, &b, &c); err == nil && n == 3 {
		return T(lang, "apply_delete_batch", a, b, c), true
	}
	for _, key := range []string{"act_disable", "act_enable", "act_delete"} {
		for _, src := range []Lang{LangZH, LangEN} {
			verb := messages[key][src]
			if verb == "" {
				continue
			}
			prefix := verb + " "
			if strings.HasPrefix(msg, prefix) {
				name := strings.TrimPrefix(msg, prefix)
				if name != "" {
					return T(lang, key) + " " + name, true
				}
			}
		}
	}
	return "", false
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
