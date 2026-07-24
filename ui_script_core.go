package main

const uiScriptCore = `  const WORKERS_MIN = 1;
  const WORKERS_MAX = 16;
  const WORKERS_DEFAULT = 6;
  const SCHEDULE_INTERVAL_MIN = 1;
  const SCHEDULE_INTERVAL_MAX = 10080;
  const state = {
    filter: 'all',
    page: 1,
    pageSize: 20,
    snapshot: { results: [], summary: {}, running: false, applying: false, done: 0, total: 0 }
  };
  const $ = (id) => document.getElementById(id);
  const prefsKey = 'grokInspectionPrefs';
  function loadPrefs() {
    try { return JSON.parse(localStorage.getItem(prefsKey) || '{}') || {}; } catch (_) { return {}; }
  }
  function savePrefs(patch) {
    localStorage.setItem(prefsKey, JSON.stringify(Object.assign(loadPrefs(), patch || {})));
  }
  function clampWorkers(n) {
    return Math.min(WORKERS_MAX, Math.max(WORKERS_MIN, n));
  }
  function parseWorkersStrict() {
    const raw = String($('workers').value == null ? '' : $('workers').value).trim();
    if (!/^\d+$/.test(raw)) {
      throw new Error(t('workers_int_prefix') + WORKERS_MIN + '-' + WORKERS_MAX + t('workers_int_mid') + WORKERS_DEFAULT + t('workers_int_suffix'));
    }
    const n = Number(raw);
    if (!Number.isInteger(n) || n < WORKERS_MIN || n > WORKERS_MAX) {
      throw new Error(t('workers_range_prefix') + WORKERS_MIN + '-' + WORKERS_MAX + t('workers_range_suffix'));
    }
    return n;
  }
  function normalizeWorkersInput(strict) {
    try {
      const n = parseWorkersStrict();
      $('workers').value = String(n);
      return n;
    } catch (e) {
      if (strict) throw e;
      $('workers').value = String(WORKERS_DEFAULT);
      return WORKERS_DEFAULT;
    }
  }
  function normalizeSamplePref(value, max) {
    const raw = String(value == null ? '' : value).trim();
    if (raw === '') return '';
    if (!/^\d+$/.test(raw)) return '';
    const n = Number(raw);
    if (!Number.isSafeInteger(n) || n < 0) return '';
    if (Number.isInteger(max) && max >= 0 && n > max) return String(max);
    return String(n);
  }
  const prefs = loadPrefs();
  state.pageSize = [20,50,100].includes(Number(prefs.pageSize)) ? Number(prefs.pageSize) : 20;
  {
    const prefWorkers = Number(prefs.workers);
    $('workers').value = String(
      Number.isInteger(prefWorkers) && prefWorkers >= WORKERS_MIN && prefWorkers <= WORKERS_MAX
        ? prefWorkers
        : WORKERS_DEFAULT
    );
  }
  $('includeDisabled').checked = !!prefs.includeDisabled;
  $('onlyDisabled').checked = !!prefs.onlyDisabled;
  if ($('onlyDisabled').checked) $('includeDisabled').checked = false;
  if ($('sampleCount')) $('sampleCount').value = normalizeSamplePref(prefs.sampleCount, 999999);
  if ($('samplePercent')) $('samplePercent').value = normalizeSamplePref(prefs.samplePercent, 100);
  const keyInput = $('managementKey');
  const KEY_STORAGE = 'grokInspectionManagementKey';
  // Match Cli-Proxy-API-Management-Center reversible localStorage obfuscation (enc::v1::).
  // Not a security boundary — same algorithm the panel uses so we can reuse its saved key.
  const PANEL_ENC_PREFIX = 'enc::v1::';
  const PANEL_SECRET_SALT = 'cli-proxy-api-webui::secure-storage';
  function panelKeyBytes() {
    try {
      return new TextEncoder().encode(PANEL_SECRET_SALT + '|' + window.location.host + '|' + navigator.userAgent);
    } catch (_) {
      return new TextEncoder().encode(PANEL_SECRET_SALT);
    }
  }
  function deobfuscatePanelValue(payload) {
    const raw = String(payload == null ? '' : payload);
    if (!raw || raw.indexOf(PANEL_ENC_PREFIX) !== 0) return raw;
    try {
      const b64 = raw.slice(PANEL_ENC_PREFIX.length);
      const binary = atob(b64);
      const encrypted = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) encrypted[i] = binary.charCodeAt(i);
      const key = panelKeyBytes();
      const out = new Uint8Array(encrypted.length);
      for (let i = 0; i < encrypted.length; i++) out[i] = encrypted[i] ^ key[i % key.length];
      return new TextDecoder().decode(out);
    } catch (_) { return raw; }
  }
  function tryParseJSON(text) {
    try { return JSON.parse(text); } catch (_) { return null; }
  }
  function storageCandidates() {
    const list = [];
    try { list.push(window.localStorage); } catch (_) {}
    try { list.push(window.sessionStorage); } catch (_) {}
    try {
      if (window.parent && window.parent !== window) {
        try { list.push(window.parent.localStorage); } catch (_) {}
        try { list.push(window.parent.sessionStorage); } catch (_) {}
      }
    } catch (_) {}
    return list;
  }
  function readStorageItem(store, name) {
    try { return store && store.getItem ? store.getItem(name) : null; } catch (_) { return null; }
  }
  function extractKeyFromPanelStorage() {
    try {
      const stores = storageCandidates();
      for (const store of stores) {
        // Zustand persist key used by Management Center auth store.
        const authRaw = readStorageItem(store, 'cli-proxy-auth');
        if (authRaw) {
          const parsed = tryParseJSON(deobfuscatePanelValue(authRaw));
          const key = (parsed && parsed.state && parsed.state.managementKey)
            || (parsed && parsed.managementKey)
            || '';
          if (String(key).trim()) return String(key).trim();
        }
        // Legacy plaintext / obfuscated single-key entries.
        for (const name of ['managementKey', 'cli-proxy-management-key', 'CPA_MANAGEMENT_KEY', 'management_password']) {
          const raw = readStorageItem(store, name);
          if (!raw) continue;
          const plain = deobfuscatePanelValue(raw);
          const parsed = tryParseJSON(plain);
          if (typeof parsed === 'string' && parsed.trim()) return parsed.trim();
          if (parsed && typeof parsed === 'object') {
            const k = parsed.managementKey || parsed.password || parsed.key || '';
            if (String(k).trim()) return String(k).trim();
          }
          if (plain && plain.indexOf(PANEL_ENC_PREFIX) !== 0 && plain.trim()) return plain.trim();
        }
      }
    } catch (_) {}
    return '';
  }
  function loadStoredManagementKey() {
    // Priority: panel remembered key, then this tab's session key.
    // Legacy plugin localStorage is read once for migration then cleared.
    const fromPanel = extractKeyFromPanelStorage();
    if (fromPanel) return fromPanel;
    try {
      const sess = sessionStorage.getItem(KEY_STORAGE) || '';
      if (sess && String(sess).trim()) return String(sess).trim();
    } catch (_) {}
    try {
      const local = localStorage.getItem(KEY_STORAGE) || '';
      if (local && String(local).trim()) {
        try { sessionStorage.setItem(KEY_STORAGE, String(local).trim()); } catch (_) {}
        try { localStorage.removeItem(KEY_STORAGE); } catch (_) {}
        return String(local).trim();
      }
    } catch (_) {}
    return '';
  }
  function persistManagementKey(value) {
    const v = String(value == null ? '' : value);
    // Session only: do not keep a second long-lived plaintext copy of the key.
    // Prefer CPA panel storage (already remembered by the management UI).
    try { sessionStorage.setItem(KEY_STORAGE, v); } catch (_) {}
    try { localStorage.removeItem(KEY_STORAGE); } catch (_) {}
  }
  let keySource = 'manual'; // panel | plugin | manual
  function applyBootKey(bootKey, sourceHint) {
    if (!bootKey || keyInput.value.trim()) return false;
    keyInput.value = bootKey;
    keySource = sourceHint || (extractKeyFromPanelStorage() === bootKey ? 'panel' : 'plugin');
    return true;
  }
  let bootKey = loadStoredManagementKey();
  applyBootKey(bootKey);
  // Panel auth store may hydrate after this iframe mounts.
  [100, 400, 1200].forEach((ms) => setTimeout(() => {
    if (keyInput.value.trim()) return;
    const again = loadStoredManagementKey();
    if (applyBootKey(again)) {
      updateAuthState();
      if (hasManagementKey()) { refresh(); loadSchedule(); }
    }
  }, ms));
  function syncKeyHint() {
    const hint = $('keyHint');
    if (!hint) return;
    if (hasManagementKey() && keySource === 'panel') {
      hint.textContent = t('key_from_panel');
      keyInput.placeholder = t('key_autofill');
    } else if (hasManagementKey() && keySource === 'plugin') {
      hint.textContent = t('key_local');
    } else if (hasManagementKey()) {
      hint.textContent = t('key_manual');
    } else {
      hint.textContent = t('key_missing');
      keyInput.placeholder = t('key_label');
    }
  }
  const hasManagementKey = () => !!keyInput.value.trim();
  const pendingOps = new Set(); // row keys currently running single-row actions
  function updateAuthState() {
    const ready = hasManagementKey();
    $('runBtn').disabled = !ready;
    if (!ready) {
      $('stopBtn').disabled = true;
      $('applyBtn').disabled = true;
      $('incrBtn').disabled = true;
      if ($('sampleBtn')) $('sampleBtn').disabled = true;
      $('batchExportBtn').disabled = true;
      $('batchDisableBtn').disabled = true;
      $('batchEnableBtn').disabled = true;
      $('batchDeleteBtn').disabled = true;
    }
    syncKeyHint();
  }
  function setProgress(text, live) {
    const el = $('progress');
    el.textContent = text || '';
    if (live) el.classList.add('live'); else el.classList.remove('live');
  }
  function showOk(msg) {
    $('error').className = 'toast-ok';
    $('error').textContent = msg || '';
  }
  function showErr(msg) {
    $('error').className = 'err';
    $('error').textContent = msg || '';
  }
  let confirmResolver = null;
  let confirmOpen = false;
  let confirmClosing = false;
  function closeConfirm(ok) {
    // Guard against duplicate close calls.
    if (confirmClosing) return;
    if (!confirmResolver && !confirmOpen) return;
    confirmClosing = true;
    const resolve = confirmResolver;
    confirmResolver = null;
    confirmOpen = false;
    try {
      const modal = document.getElementById('confirmModal');
      if (modal) {
        modal.classList.add('hidden');
        modal.setAttribute('aria-hidden', 'true');
      }
    } catch (_) {}
    // Resolve in microtask so UI hide is not blocked by await-chain work.
    const finished = () => {
      confirmClosing = false;
      if (resolve) {
        try { resolve(!!ok); } catch (_) {}
      }
      // Resume poll if a job is still running (inspect side).
      try {
        if (typeof syncPolling === 'function') syncPolling(state && state.snapshot);
      } catch (_) {}
    };
    try {
      if (typeof queueMicrotask === 'function') queueMicrotask(finished);
      else setTimeout(finished, 0);
    } catch (_) {
      finished();
    }
  }
  function bindConfirmAction(el, value) {
    if (!el) return;
    el.onpointerdown = null;
    el.onclick = (ev) => {
      try {
        if (ev) {
          ev.preventDefault();
          ev.stopPropagation();
          if (ev.stopImmediatePropagation) ev.stopImmediatePropagation();
        }
      } catch (_) {}
      closeConfirm(value);
    };
  }
  function confirmDialog(title, message, opts) {
    opts = opts || {};
    const showCancel = opts.showCancel !== false;
    return new Promise((resolve) => {
      // Drop any previous pending dialog as cancel to avoid stuck promises.
      if (confirmResolver) {
        const prev = confirmResolver;
        confirmResolver = null;
        try { prev(false); } catch (_) {}
      }
      const modal = document.getElementById('confirmModal');
      const titleEl = document.getElementById('confirmTitle');
      const msgEl = document.getElementById('confirmMsg');
      const okEl = document.getElementById('confirmOk');
      const cancelEl = document.getElementById('confirmCancel');
      if (!modal || !titleEl || !msgEl || !okEl) {
        if (!showCancel) {
          window.alert((title ? title + '\n' : '') + (message || ''));
          resolve(true);
          return;
        }
        resolve(window.confirm((title ? title + '\n' : '') + (message || '')));
        return;
      }
      confirmResolver = resolve;
      confirmOpen = true;
      confirmClosing = false;
      // Pause inspect polling while modal is open so cancel is not delayed by render().
      try { if (typeof stopPolling === 'function') stopPolling(); } catch (_) {}
      titleEl.textContent = title || t('confirm_title');
      msgEl.textContent = message || '';
      if (cancelEl) cancelEl.style.display = showCancel ? '' : 'none';
      bindConfirmAction(okEl, true);
      bindConfirmAction(cancelEl, false);
      modal.onpointerdown = null;
      modal.onclick = (ev) => {
        if (ev.target === modal) closeConfirm(false);
      };
      const card = modal.querySelector('.modal-card');
      if (card) {
        card.onpointerdown = (ev) => { try { ev.stopPropagation(); } catch (_) {} };
        card.onclick = (ev) => { try { ev.stopPropagation(); } catch (_) {} };
      }
      modal.classList.remove('hidden');
      modal.setAttribute('aria-hidden', 'false');
      try {
        const focusEl = showCancel && cancelEl ? cancelEl : okEl;
        if (focusEl && focusEl.focus) focusEl.focus({ preventScroll: true });
      } catch (_) {
        try { (showCancel && cancelEl ? cancelEl : okEl).focus(); } catch (_) {}
      }
    });
  }
  document.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape' && ev.key !== 'Esc') return;
    const modal = document.getElementById('confirmModal');
    if (!modal || modal.classList.contains('hidden') || !confirmOpen) return;
    try { ev.preventDefault(); } catch (_) {}
    closeConfirm(false);
  });
  function classificationsForFilter(filter) {
    if (!filter || filter === 'all') return [];
    // Server expands "other" to non-primary classes.
    return [filter];
  }
`
