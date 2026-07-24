package main

const uiScriptLang = `
  const LANG_KEY = 'grok-inspection.lang';
  function detectLang() {
    try {
      const saved = localStorage.getItem(LANG_KEY);
      if (saved === 'zh' || saved === 'en') return saved;
    } catch (e) {}
    return 'zh';
  }
  let lang = detectLang();
  function t(key) {
    const pack = I18N[lang] || I18N.zh;
    return (pack && pack[key]) || (I18N.zh && I18N.zh[key]) || key;
  }
  function applyStaticI18n() {
    document.documentElement.lang = (lang === 'en') ? 'en' : 'zh-CN';
    document.querySelectorAll('[data-i18n]').forEach((el) => {
      const key = el.getAttribute('data-i18n');
      if (!key) return;
      const val = t(key);
      if (el.tagName === 'TITLE') document.title = val;
      el.textContent = val;
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach((el) => {
      const key = el.getAttribute('data-i18n-placeholder');
      if (!key) return;
      el.setAttribute('placeholder', t(key));
    });
    document.querySelectorAll('[data-i18n-title]').forEach((el) => {
      const key = el.getAttribute('data-i18n-title');
      if (!key) return;
      el.setAttribute('title', t(key));
    });
    document.querySelectorAll('[data-i18n-aria-label]').forEach((el) => {
      const key = el.getAttribute('data-i18n-aria-label');
      if (!key) return;
      el.setAttribute('aria-label', t(key));
    });
    const sel = document.getElementById('langSelect');
    if (sel) sel.value = lang;
    if (typeof classLabel !== 'undefined') {
      classLabel.healthy = t('class_healthy');
      classLabel.permission_denied = t('class_permission_denied');
      classLabel.quota_exhausted = t('class_quota_exhausted');
      classLabel.spending_limit = t('class_spending_limit');
      classLabel.reauth = t('class_reauth');
      classLabel.model_unavailable = t('class_model_unavailable');
      classLabel.probe_error = t('class_probe_error');
      classLabel.unknown = t('class_unknown');
    }
    if (typeof actionLabel !== 'undefined') {
      actionLabel.keep = t('action_keep');
      actionLabel.disable = t('action_disable');
      actionLabel.enable = t('action_enable');
      actionLabel.delete = t('action_delete');
    }
  }

  // Operator-facing probe reasons stored from the Go runtime. Keep catalogs aligned with i18n.go.
  const REASON_I18N = {
    zh: {
      auth_expired:'认证已过期或失效',
      quota_exhausted:'额度已用尽',
      temp_rate_limited:'临时限流 (HTTP 429)，建议稍后重试',
      permission_denied:'对话权限被拒绝',
      spending_limit:'额度或订阅受限',
      model_unavailable:'测试模型不可用',
      chat_ok:'对话测试成功',
      probe_failed:'探测失败',
      unable_classify:'无法可靠分类',
      stopped_before_probe:'已停止，未探测',
      stopped:'已停止',
      account_missing:'Auth 列表中已不存在该账号',
      missing_auth_index:'缺少 auth_index',
      fallback_disagreed:'；备用接口结果不一致，按主探测结果判定',
      list_accounts_timeout:'列出账号超时（30s）',
      list_accounts_failed_prefix:'列出账号失败: ',
      probe_timeout_prefix:'探测超时（>',
      probe_timeout_suffix:'）',
      http_probe_timeout_head:'HTTP 探测超时（',
      http_probe_timeout_mid:'）: '
    },
    en: {
      auth_expired:'Authentication expired or invalid',
      quota_exhausted:'Quota exhausted',
      temp_rate_limited:'Temporarily rate-limited (HTTP 429); retry later',
      permission_denied:'Chat permission denied',
      spending_limit:'Spending or subscription limit',
      model_unavailable:'Probe model unavailable',
      chat_ok:'Chat probe succeeded',
      probe_failed:'Probe failed',
      unable_classify:'Unable to classify reliably',
      stopped_before_probe:'Stopped before probing',
      stopped:'Stopped',
      account_missing:'Account no longer exists in the Auth list',
      missing_auth_index:'Missing auth_index',
      fallback_disagreed:'; fallback endpoint disagreed; using primary probe result',
      list_accounts_timeout:'Listing accounts timed out (30s)',
      list_accounts_failed_prefix:'Failed to list accounts: ',
      probe_timeout_prefix:'Probe timed out (>',
      probe_timeout_suffix:')',
      http_probe_timeout_head:'HTTP probe timed out (',
      http_probe_timeout_mid:'): '
    }
  };
  function reasonText(key) {
    const pack = REASON_I18N[lang] || REASON_I18N.zh;
    return (pack && pack[key]) || (REASON_I18N.zh && REASON_I18N.zh[key]) || key;
  }
  function formatHTTPProbeTimeout(dur, method, url) {
    return reasonText('http_probe_timeout_head') + dur + reasonText('http_probe_timeout_mid') + method + ' ' + url;
  }
  function formatProbeTimeout(dur) {
    return reasonText('probe_timeout_prefix') + dur + reasonText('probe_timeout_suffix');
  }
  function formatListAccountsFailed(detail) {
    return reasonText('list_accounts_failed_prefix') + detail;
  }
  function localizeKnownReason(reason) {
    const original = (reason == null) ? '' : String(reason);
    reason = original.trim();
    if (!reason) return original;
    const catalogs = [REASON_I18N.zh, REASON_I18N.en];
    const skip = {
      fallback_disagreed:1, list_accounts_timeout:1, list_accounts_failed_prefix:1,
      probe_timeout_prefix:1, probe_timeout_suffix:1,
      http_probe_timeout_head:1, http_probe_timeout_mid:1
    };
    // Formatted: HTTP probe timeout
    for (const cat of catalogs) {
      const head = cat.http_probe_timeout_head;
      const mid = cat.http_probe_timeout_mid;
      if (head && mid && reason.indexOf(head) === 0) {
        const rest = reason.slice(head.length);
        const midIdx = rest.indexOf(mid);
        if (midIdx > 0) {
          const dur = rest.slice(0, midIdx);
          const tail = rest.slice(midIdx + mid.length).trim();
          const sp = tail.indexOf(' ');
          if (sp > 0) {
            return formatHTTPProbeTimeout(dur, tail.slice(0, sp), tail.slice(sp + 1));
          }
        }
      }
    }
    // Formatted: account probe timeout
    for (const cat of catalogs) {
      const head = cat.probe_timeout_prefix;
      const suf = cat.probe_timeout_suffix;
      if (head && suf && reason.indexOf(head) === 0 && reason.slice(-suf.length) === suf) {
        const dur = reason.slice(head.length, reason.length - suf.length);
        return formatProbeTimeout(dur);
      }
    }
    // Formatted: list accounts failed (detail may itself be a known reason, e.g. list timeout)
    for (const cat of catalogs) {
      const prefix = cat.list_accounts_failed_prefix;
      if (prefix && reason.indexOf(prefix) === 0) {
        const detail = localizeKnownReason(reason.slice(prefix.length));
        return formatListAccountsFailed(detail);
      }
    }
    const keys = Object.keys(REASON_I18N.zh).filter((k) => !skip[k]);
    for (const key of keys) {
      for (const cat of catalogs) {
        const candidate = cat[key];
        if (!candidate) continue;
        if (reason === candidate) return reasonText(key);
        const prefix = candidate + ' (HTTP ';
        if (reason.indexOf(prefix) === 0 && reason.charAt(reason.length - 1) === ')') {
          return reasonText(key) + reason.slice(candidate.length);
        }
        for (const catSuf of catalogs) {
          const suf = catSuf.fallback_disagreed;
          if (!suf || reason.length < suf.length || reason.slice(-suf.length) !== suf) continue;
          const base = reason.slice(0, reason.length - suf.length);
          if (base === candidate) return reasonText(key) + reasonText('fallback_disagreed');
          if (base.indexOf(candidate + ' (HTTP ') === 0) {
            return reasonText(key) + base.slice(candidate.length) + reasonText('fallback_disagreed');
          }
        }
      }
    }
    // exact list_accounts_timeout / other whole strings
    for (const cat of catalogs) {
      for (const key of Object.keys(cat)) {
        if (reason === cat[key] && !skip[key]) return reasonText(key);
        if (key === 'list_accounts_timeout' && reason === cat[key]) return reasonText(key);
      }
    }
    // Unknown free-form diagnostics keep original leading/trailing whitespace.
    return original;
  }

  function localizeKnownActionError(msg) {
    const original = (msg == null) ? '' : String(msg);
    msg = original.trim();
    if (!msg) return original;
    const catalogs = [REASON_I18N.zh, REASON_I18N.en];
    function matchWhole(m) {
      // exact stopped / list timeout / auth file name missing / mgmt password
      const exactKeys = ['stopped', 'list_accounts_timeout'];
      for (const key of exactKeys) {
        for (const cat of catalogs) {
          if (cat[key] && m === cat[key]) return reasonText(key);
        }
      }
      // enable/unban superseded by concurrent ban (sentinel Error() text)
      const banConflict = {
        en: 'ban_conflict: concurrent ban retained',
        enLegacy: 'unban_conflict: concurrent ban retained',
        zh: '启用被并发自动禁用抢占，账号已重新禁用',
        enOut: 'enable superseded by concurrent ban; account re-disabled'
      };
      if (m === banConflict.en || m === banConflict.enLegacy || m === banConflict.zh || m === banConflict.enOut) {
        return lang === 'en' ? banConflict.enOut : banConflict.zh;
      }
      // auth file name missing (exact, no account)
      const missing = {
        zh: '账号缺少 auth 文件名',
        en: 'auth file name missing'
      };
      if (m === missing.zh || m === missing.en) {
        return lang === 'en' ? missing.en : missing.zh;
      }
      // management password (prefix)
      const pw = {
        zh: '管理密码不可用',
        en: 'CPA management password is unavailable'
      };
      for (const base of [pw.en, pw.zh]) {
        if (m === base || m.indexOf(base + ' ') === 0 || m.indexOf(base + '(') === 0 || m.indexOf(base + ' (') === 0) {
          return lang === 'en' ? pw.en : pw.zh;
        }
      }
      // prefixed patterns: head + free detail
      const prefixes = [
        { zh: '未找到账号: ', en: 'auth not found: ', outZh: '未找到账号: ', outEn: 'auth not found: ' },
        { zh: '账号缺少 auth 文件名: ', en: 'auth file name missing for ', outZh: '账号缺少 auth 文件名: ', outEn: 'auth file name missing for ' },
        { zh: '已在 CPA 启用但保存禁用状态失败: ', en: 'enabled in CPA but failed to persist ban state: ', outZh: '已在 CPA 启用但保存禁用状态失败: ', outEn: 'enabled in CPA but failed to persist ban state: ' },
        { zh: '本地已删除但保存禁用状态失败: ', en: 'deleted locally but failed to persist ban state: ', outZh: '本地已删除但保存禁用状态失败: ', outEn: 'deleted locally but failed to persist ban state: ' },
        { zh: '已在 CPA 删除但保存禁用状态失败: ', en: 'deleted in CPA but failed to persist ban state: ', outZh: '已在 CPA 删除但保存禁用状态失败: ', outEn: 'deleted in CPA but failed to persist ban state: ' },
        { zh: '已在 CPA 解禁但保存禁用状态失败: ', en: 'unbanned in CPA but failed to persist ban state: ', outZh: '已在 CPA 解禁但保存禁用状态失败: ', outEn: 'unbanned in CPA but failed to persist ban state: ' },
        { zh: '保存禁用状态: ', en: 'persist ban state: ', outZh: '保存禁用状态: ', outEn: 'persist ban state: ' },
        { zh: '保存自动禁用状态失败: ', en: 'Failed to save auto-ban state: ', outZh: '保存自动禁用状态失败: ', outEn: 'Failed to save auto-ban state: ' }
      ];
      for (const p of prefixes) {
        for (const head of [p.zh, p.en]) {
          if (head && m.indexOf(head) === 0) {
            const detail = m.slice(head.length);
            return (lang === 'en' ? p.outEn : p.outZh) + detail;
          }
        }
      }
      // unsupported action "x"
      for (const u of [{zh:'不支持的操作 "', en:'unsupported action "'}]) {
        for (const head of [u.zh, u.en]) {
          if (m.indexOf(head) === 0 && m.charAt(m.length - 1) === '"') {
            const action = m.slice(head.length, m.length - 1);
            return (lang === 'en' ? u.en : u.zh) + action + '"';
          }
        }
      }
      return null;
    }
    let whole = matchWhole(msg);
    if (whole != null) return whole;
    // account prefix: "acct: known"
    const idx = msg.indexOf(': ');
    if (idx > 0) {
      const left = msg.slice(0, idx);
      const right = msg.slice(idx + 2).trim();
      const lower = left.toLowerCase();
      const looksAccount = left && left.indexOf(' ') < 0 && left.indexOf('://') < 0 &&
        lower.indexOf('http') !== 0 &&
        lower.indexOf('auth') < 0 && lower.indexOf('password') < 0 &&
        lower.indexOf('persist') < 0 && lower.indexOf('failed') < 0 &&
        left.indexOf('失败') < 0 && left.indexOf('未找到') < 0 && left.indexOf('缺少') < 0;
      if (looksAccount) {
        const rest = matchWhole(right);
        if (rest != null) return left + ': ' + rest;
      }
    }
    // reuse reason localizer for Stopped / timeouts / etc.
    const viaReason = localizeKnownReason(msg);
    if (viaReason !== msg) return viaReason;
    // Unknown free-form diagnostics keep original leading/trailing whitespace.
    return original;
  }

  function setLang(next) {
    lang = (next === 'en') ? 'en' : 'zh';
    try { localStorage.setItem(LANG_KEY, lang); } catch (e) {}
    applyStaticI18n();
    try { syncKeyHint(); } catch (e) {}
    try { render(); } catch (e) {}
    try { if (typeof renderBanPage === 'function') renderBanPage(); } catch (e) {}
    try { if (typeof switchTab === 'function') switchTab(currentTab); } catch (e) {}
  }

    function namedTheme(value) {
    const text = String(value || '').trim().toLowerCase();
    const normalized = text.replace(/[_-]+/g, ' ');
    const tokens = normalized.split(/\s+/).filter(Boolean);
    const darkNames = ['dark', 'night', 'black'];
    const lightNames = ['light', 'white', 'day', 'bright', 'default'];
    if (darkNames.some((name) => tokens.includes(name))) return 'dark';
    if (lightNames.some((name) => tokens.includes(name))) return 'light';
    return '';
  }
  function elementTheme(el) {
    if (!el) return '';
    for (const name of ['data-theme', 'data-color-scheme', 'data-mode', 'data-bs-theme']) {
      const theme = namedTheme(el.getAttribute && el.getAttribute(name));
      if (theme) return theme;
    }
    return namedTheme(el.className);
  }
  function backgroundTheme(doc) {
    if (!doc || !doc.defaultView) return '';
    for (const el of [doc.body, doc.documentElement]) {
      if (!el) continue;
      const color = doc.defaultView.getComputedStyle(el).backgroundColor || '';
      const match = color.match(/rgba?\(([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\)/i);
      if (!match || (match[4] != null && Number(match[4]) < 0.2)) continue;
      const rgb = [Number(match[1]), Number(match[2]), Number(match[3])].map((value) => {
        const n = value / 255;
        return n <= 0.04045 ? n / 12.92 : Math.pow((n + 0.055) / 1.055, 2.4);
      });
      const luminance = 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
      return luminance < 0.32 ? 'dark' : 'light';
    }
    return '';
  }
  function prefersDark() {
    try { return !!(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches); } catch (_) { return false; }
  }
  function detectHostTheme() {
    try {
      if (window.parent && window.parent !== window) {
        const doc = window.parent.document;
        const fromParent = elementTheme(doc.documentElement) || elementTheme(doc.body) || backgroundTheme(doc);
        if (fromParent) return fromParent;
      }
    } catch (_) {}
    const local = elementTheme(document.documentElement) || elementTheme(document.body) || backgroundTheme(document);
    if (local) return local;
    return prefersDark() ? 'dark' : 'light';
  }
  function syncHostTheme() {
    const theme = detectHostTheme() || 'light';
    document.documentElement.setAttribute('data-grok-theme', theme);
    try {
      if (document.body) document.body.setAttribute('data-grok-theme', theme);
      const page = document.querySelector('.grok-inspection-page');
      if (page) page.setAttribute('data-grok-theme', theme);
    } catch (_) {}
  }
  syncHostTheme();
  // Host SPA may paint theme class after iframe load.
  setTimeout(syncHostTheme, 50);
  setTimeout(syncHostTheme, 300);
  setTimeout(syncHostTheme, 1000);
  try {
    if (window.parent && window.parent !== window) {
      const parentDoc = window.parent.document;
      const themeObserver = new MutationObserver(syncHostTheme);
      const themeTargets = [parentDoc.documentElement, parentDoc.body].filter(Boolean);
      for (const target of themeTargets) {
        themeObserver.observe(target, {
          attributes: true,
          attributeFilter: ['class', 'style', 'data-theme', 'data-color-scheme', 'data-mode', 'data-bs-theme']
        });
      }
    }
  } catch (_) {}
  try {
    if (window.matchMedia) {
      const mq = window.matchMedia('(prefers-color-scheme: dark)');
      if (mq.addEventListener) mq.addEventListener('change', syncHostTheme);
      else if (mq.addListener) mq.addListener(syncHostTheme);
    }
  } catch (_) {}
  window.addEventListener('pageshow', syncHostTheme);
  window.addEventListener('focus', syncHostTheme);
  window.addEventListener('storage', syncHostTheme);

`
