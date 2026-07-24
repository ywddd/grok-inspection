package main

const uiScriptBan = `  function heroTextFor(tab) {
    return tab === 'autoban' ? t('hero_autoban') : t('subtitle');
  }
  let banState = { bans: [], page: 1, pageSize: 20, meta: {}, filter: 'all' };
  let currentTab = 'inspect';
  function switchTab(name) {
    if (name !== 'inspect' && name !== 'autoban') name = 'inspect';
    currentTab = name;
    document.querySelectorAll('.tab').forEach((t) => {
      const on = t.dataset.tab === name;
      t.classList.toggle('active', on);
      t.setAttribute('aria-selected', on ? 'true' : 'false');
      t.tabIndex = on ? 0 : -1;
    });
    document.querySelectorAll('.panel').forEach((p) => p.classList.toggle('active', p.id === 'panel-' + name));
    const sub = document.getElementById('heroSub');
    if (sub) sub.textContent = heroTextFor(name) || heroTextFor('inspect');
    if (name === 'autoban') loadBans();
  }
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', (ev) => {
      ev.preventDefault();
      switchTab(tab.dataset.tab);
    });
  });
  // Default: 账号巡检 selected and sticky after blur/clicks elsewhere.
  switchTab('inspect');

  function formatRemain(sec) {
    sec = Number(sec || 0);
    if (!Number.isFinite(sec) || sec <= 0) return '—';
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (h >= 24 * 30) return t('ban_manual');
    if (h > 0) return h + t('hours_minutes_mid') + String(m).padStart(2, '0') + t('hours_minutes_suffix');
    return m + t('minutes_suffix');
  }
  function formatBanReason(code) {
    const c = String(code || '').trim().toLowerCase();
    if (!c) return t('ban_unknown_reason');
    if (c === 'subscription:free-usage-exhausted' || c.indexOf('free-usage-exhausted') >= 0) return t('ban_reason_quota');
    if (c === 'personal-team-blocked:spending-limit' || c.indexOf('spending-limit') >= 0) return t('ban_spending_limit');
    if (c === 'permission-denied' || c.indexOf('permission-denied') >= 0) return t('ban_reason_permission');
    if (c === 'unauthorized' || c === '401' || c.indexOf('unauthorized') >= 0) return t('ban_reason_authfail');
    if (c === 'manual-disabled' || c.indexOf('manual-disabled') >= 0) return t('ban_reason_manual_disabled');
    return code;
  }
  function formatResetSource(source, remainingSec) {
    const s = String(source || '').trim().toLowerCase();
    const sec = Number(remainingSec || 0);
    if (s === 'manual_unban' || (Number.isFinite(sec) && sec > 24 * 30 * 3600)) return t('ban_manual');
    if (s === 'header_absolute') return t('restore_header_absolute');
    if (s === 'header_relative') return t('restore_header_relative');
    if (s === 'date_plus_fallback') return t('restore_date_plus_fallback');
    if (s === 'local_plus_fallback') return t('restore_local_plus_fallback');
    if (!s) return t('ban_auto_restore');
    return source;
  }
  function formatShanghaiTime(raw) {
    if (!raw) return '—';
    const d = new Date(raw);
    if (Number.isNaN(d.getTime())) {
      return String(raw).replace('T', ' ').replace('Z', ' UTC');
    }
    try {
      return new Intl.DateTimeFormat('zh-CN', {
        timeZone: 'Asia/Shanghai',
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit',
        hour12: false
      }).format(d);
    } catch (_) {
      // fallback: UTC+8
      const sh = new Date(d.getTime() + 8 * 3600 * 1000);
      const p = (n) => String(n).padStart(2, '0');
      return sh.getUTCFullYear() + '-' + p(sh.getUTCMonth()+1) + '-' + p(sh.getUTCDate()) +
        ' ' + p(sh.getUTCHours()) + ':' + p(sh.getUTCMinutes()) + ':' + p(sh.getUTCSeconds());
    }
  }
  function banCategoryOf(b) {
    if (!b) return 'other';
    if (b.category) return String(b.category);
    const c = String(b.error_code || '').trim().toLowerCase();
    if (!c) return 'other';
    if (c === 'subscription:free-usage-exhausted' || c.indexOf('free-usage-exhausted') >= 0) return 'quota';
    if (c === 'personal-team-blocked:spending-limit' || c.indexOf('spending-limit') >= 0) return 'spending_limit';
    if (c === 'permission-denied' || c.indexOf('permission-denied') >= 0) return 'permission';
    if (c === 'unauthorized' || c === '401' || c.indexOf('unauthorized') >= 0 || c.indexOf('authentication_error') >= 0 || c.indexOf('invalid_token') >= 0 || c.indexOf('token_expired') >= 0 || c === 'unauthenticated') return 'unauthorized';
    if (c === 'manual-disabled' || c.indexOf('manual-disabled') >= 0) return 'manual';
    return 'other';
  }
  function banFilterLabel(f) {
    if (f === 'quota') return t('ban_quota');
    if (f === 'spending_limit') return t('ban_spending_limit');
    if (f === 'permission') return t('ban_permission');
    if (f === 'unauthorized') return t('ban_authfail');
    if (f === 'manual') return t('ban_manual_disabled');
    return t('ban_all');
  }
  function filteredBans() {
    const list = Array.isArray(banState.bans) ? banState.bans : [];
    const f = banState.filter || 'all';
    if (f === 'all') return list;
    return list.filter((b) => banCategoryOf(b) === f);
  }
  function syncBanFilterUI() {
    const f = banState.filter || 'all';
    document.querySelectorAll('[data-ban-filter]').forEach((card) => {
      card.classList.toggle('active', card.getAttribute('data-ban-filter') === f);
    });
    const filtered = filteredBans();
    const btn = document.getElementById('banUnbanFilterBtn');
    if (btn) {
      const n = filtered.length;
      btn.disabled = n === 0;
      btn.textContent = f === 'all'
        ? (t('ban_unban_filter') + (n ? ' (' + n + ')' : ''))
        : (t('ban_unban_filter_named_prefix') + banFilterLabel(f) + t('ban_unban_filter_named_suffix') + (n ? ' (' + n + ')' : ''));
    }
    const hint = document.getElementById('banFilterHint');
    if (hint) {
      hint.textContent = f === 'all'
        ? t('ban_filter_hint_all')
        : (t('ban_filter_current_prefix') + banFilterLabel(f) + t('ban_filter_current_mid') + filtered.length + t('ban_filter_current_suffix'));
    }
  }
  function setBanFilter(f) {
    banState.filter = f || 'all';
    banState.page = 1;
    renderBanPage();
  }
  function renderBanPage() {
    const list = filteredBans();
    const size = [20,50,100].includes(Number(banState.pageSize)) ? Number(banState.pageSize) : 20;
    const pages = Math.max(1, Math.ceil(list.length / size));
    if (banState.page > pages) banState.page = pages;
    if (banState.page < 1) banState.page = 1;
    const start = (banState.page - 1) * size;
    const slice = list.slice(start, start + size);
    const rows = document.getElementById('banRows');
    const empty = document.getElementById('banEmpty');
    const pager = document.getElementById('banPager');
    if (!rows) return;
    rows.innerHTML = slice.map((b) => {
      const id = String(b.auth_id || '');
      const synced = !!b.cpa_synced;
      let syncLabel = synced ? t('ban_synced') : t('ban_unsynced');
      const syncErr = String(b.cpa_sync_error || '').trim();
      if (!synced && syncErr) syncLabel += ' · ' + syncErr;
      return '<tr>' +
        '<td class="col-name">' + esc(id) + '</td>' +
        '<td>' + esc(formatBanReason(b.error_code)) + '</td>' +
        '<td>' + esc(formatShanghaiTime(b.banned_at)) + '</td>' +
        '<td>' + esc(formatResetSource(b.reset_source, b.remaining_seconds)) + '</td>' +
        '<td>' + esc(formatRemain(b.remaining_seconds)) + '</td>' +
        '<td>' + esc(syncLabel) + '</td>' +
        '<td><button type="button" data-unban="' + esc(id).replace(/"/g, '&quot;') + '">' + t('ban_unban') + '</button></td>' +
      '</tr>';
    }).join('');
    rows.querySelectorAll('[data-unban]').forEach((btn) => {
      btn.onclick = () => unbanOne(btn.getAttribute('data-unban'));
    });
    if (empty) {
      if (list.length) {
        empty.style.display = 'none';
        empty.textContent = '';
      } else {
        empty.style.display = 'block';
        empty.textContent = hasManagementKey()
          ? ((banState.filter && banState.filter !== 'all')
            ? (t('ban_empty_filter_prefix') + banFilterLabel(banState.filter) + t('ban_empty_filter_suffix'))
            : t('ban_empty'))
          : t('ban_need_key_load');
      }
    }
    if (pager) {
      pager.innerHTML =
        '<div class="pager-meta pager-meta-row">' + t('pager_total_prefix') + list.length + t('pager_total_suffix') +
        ((banState.filter && banState.filter !== 'all') ? (t('pager_filter_prefix') + banFilterLabel(banState.filter) + t('pager_filter_suffix')) : '') +
        t('pager_page_mid') + banState.page + t('pager_page_of') + pages + t('pager_page_suffix') +
        t('per_page') + '<select id="banPageSize">' +
        [20,50,100].map((n) => '<option value="' + n + '"' + (size===n?' selected':'') + '>' + n + '</option>').join('') +
        '</select></div>' +
        '<div style="display:flex;gap:8px;align-items:center">' +
        '<button id="banPrev"' + (banState.page<=1?' disabled':'') + '>' + t('prev') + '</button>' +
        '<button id="banNext"' + (banState.page>=pages?' disabled':'') + '>' + t('next') + '</button></div>';
      const banPageSizeEl = document.getElementById('banPageSize');
      if (banPageSizeEl) {
        banPageSizeEl.onchange = () => {
          banState.pageSize = Number(banPageSizeEl.value) || 20;
          savePrefs({ banPageSize: banState.pageSize });
          banState.page = 1;
          renderBanPage();
        };
      }
      const prev = document.getElementById('banPrev');
      const next = document.getElementById('banNext');
      if (prev) prev.onclick = () => { if (banState.page > 1) { banState.page--; renderBanPage(); } };
      if (next) next.onclick = () => { if (banState.page < pages) { banState.page++; renderBanPage(); } };
    }
    syncBanFilterUI();
  }
async function loadBans() {
    const errEl = document.getElementById('banError');
    if (errEl) errEl.textContent = '';
    if (!hasManagementKey()) {
      banState.bans = [];
      banState.meta = {};
      const toggle = document.getElementById('banEnabledToggle');
      if (toggle) { toggle.disabled = true; toggle.checked = false; }
      const pill = document.getElementById('banEnabledPill');
      if (pill) { pill.textContent = t('ban_not_loaded'); pill.className = 'status-pill off'; }
      renderBanPage();
      return;
    }
    try {
      const raw = await api('/bans');
      const data = (raw && typeof raw === 'object' && raw.bans == null && raw.result && typeof raw.result === 'object')
        ? raw.result
        : (raw || {});
      const list = Array.isArray(data.bans) ? data.bans
        : (Array.isArray(data.items) ? data.items : []);
      banState.bans = list;
      banState.meta = data;
      banState.page = 1;
      const set = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = String(v); };
            // counts by category (prefer server, fallback client-side)
      let q = 0, s = 0, p = 0, u = 0, m = 0;
      banState.bans.forEach((b) => {
        const c = banCategoryOf(b);
        if (c === 'quota') q++;
        else if (c === 'spending_limit') s++;
        else if (c === 'permission') p++;
        else if (c === 'unauthorized') u++;
        else if (c === 'manual') m++;
      });
      set('banCount', data.count != null ? data.count : banState.bans.length);
      set('banQuotaCount', data.quota_count != null ? data.quota_count : q);
      set('banSpendingLimitCount', data.spending_limit_count != null ? data.spending_limit_count : s);
      set('banPermissionCount', data.permission_count != null ? data.permission_count : p);
      set('banUnauthorizedCount', data.unauthorized_count != null ? data.unauthorized_count : u);
      const unsynced = data.unsynced_count != null ? Number(data.unsynced_count) : banState.bans.filter((b)=>!b.cpa_synced).length;
      const banner = document.getElementById('banUnsyncedBanner');
      if (banner) {
        if (unsynced > 0) {
          banner.style.display = 'block';
          banner.textContent = t('ban_unsynced_banner_prefix') + unsynced + t('ban_unsynced_banner_suffix');
        } else {
          banner.style.display = 'none';
          banner.textContent = '';
        }
      }
      set('banManualDisabledCount', data.manual_disabled_count != null ? data.manual_disabled_count : m);
      const on = data.enabled !== false;
      const toggle = document.getElementById('banEnabledToggle');
      if (toggle) {
        toggle.disabled = false;
        toggle.checked = on;
      }
      const pill = document.getElementById('banEnabledPill');
      if (pill) {
        pill.textContent = on ? t('ban_running') : t('ban_off');
        pill.className = 'status-pill ' + (on ? 'on' : 'off');
      }
      renderBanPage();
    } catch (e) {
      if (errEl) errEl.textContent = String(e.message || e);
      banState.bans = [];
      renderBanPage();
    }
  }
      async function unbanOne(id) {
    if (!id || unbanOne._busy) return;
    unbanOne._busy = true;
    try {
      const ok = await confirmDialog(t('unban_confirm_title'), t('unban_confirm_body_prefix') + id);
      if (!ok) return;
      const raw = await api('/unban', { method: 'POST', body: JSON.stringify({ auth_id: id }) });
      const data = (raw && raw.result && typeof raw.result === 'object') ? raw.result : (raw || {});
      if (data && data.ok === false) throw new Error(data.error || t('unban_failed'));
      const msg = data && data.missing
        ? t('unban_missing')
        : t('unban_success_msg');
      await confirmDialog(t('unban_success_title'), msg, { showCancel: false });
      await loadBans();
    } catch (e) {
      try { await confirmDialog(t('unban_failed'), String(e.message || e), { showCancel: false }); } catch (_) {}
      const errEl = document.getElementById('banError');
      if (errEl) errEl.textContent = String(e.message || e);
    } finally {
      unbanOne._busy = false;
    }
  }
    async function unbanCurrentFilter() {
    if (unbanCurrentFilter._busy) return;
    unbanCurrentFilter._busy = true;
    const errEl = document.getElementById('banError');
    if (errEl) errEl.textContent = '';
    const btn = document.getElementById('banUnbanFilterBtn');
    try {
      const list = filteredBans();
      if (!list.length) {
        await confirmDialog(t('notice_title'), t('unban_filter_empty'), { showCancel: false });
        return;
      }
      const label = banFilterLabel(banState.filter || 'all');
      const ok = await confirmDialog(
        t('unban_filter_confirm_title'),
        t('unban_filter_confirm_body_prefix') + label + t('unban_filter_confirm_body_mid') + list.length + t('unban_filter_confirm_body_suffix')
      );
      if (!ok) return;
      if (btn) { btn.disabled = true; btn.textContent = t('unban_in_progress'); }
      const ids = list.map((b) => String(b.auth_id || '').trim()).filter(Boolean);
      const body = (banState.filter && banState.filter !== 'all')
        ? { category: banState.filter, auth_ids: ids }
        : { auth_ids: ids };
      const data = await api('/unban-all', { method: 'POST', body: JSON.stringify(body) });
      if (data && data.ok === false) throw new Error(data.error || t('unban_start_failed'));
      showOk(t('unban_filter_started_prefix') + ids.length + t('unban_filter_started_suffix'));
      startPolling();
      await refresh({ light: true });
      await loadBans();
    } catch (e) {
      try { await confirmDialog(t('unban_failed'), String(e.message || e), { showCancel: false }); } catch (_) {}
      if (errEl) errEl.textContent = String(e.message || e);
    } finally {
      unbanCurrentFilter._busy = false;
      syncBanFilterUI();
    }
  }
  async function unbanAll() {
    const ok = await confirmDialog(t('unban_all_confirm_title'), t('unban_all_confirm_body'));
    if (!ok) return;
    try {
      const data = await api('/unban-all', { method: 'POST', body: '{}' });
      if (data && data.ok === false) throw new Error(data.error || t('unban_start_failed'));
      showOk(t('unban_all_started'));
      startPolling();
      await refresh({ light: true });
      await loadBans();
    } catch (e) {
      await confirmDialog(t('notice_title'), String(e.message || e));
    }
  }
async function setAutobanEnabled(on) {
    const errEl = document.getElementById('banError');
    if (errEl) errEl.textContent = '';
    const toggle = document.getElementById('banEnabledToggle');
    if (toggle) toggle.disabled = true;
    try {
      const data = await api('/autoban-settings', { method: 'POST', body: JSON.stringify({ enabled: !!on }) });
      const enabled = data && data.enabled !== false;
      if (toggle) toggle.checked = enabled;
      const pill = document.getElementById('banEnabledPill');
      if (pill) {
        pill.textContent = enabled ? t('ban_running') : t('ban_off');
        pill.className = 'status-pill ' + (enabled ? 'on' : 'off');
      }
      const label = document.getElementById('banEnabledLabel');
      if (label) label.textContent = enabled ? t('ban_running') : t('ban_off');
    } catch (e) {
      if (errEl) errEl.textContent = String(e.message || e);
      if (toggle) toggle.checked = !on;
    } finally {
      if (toggle) toggle.disabled = false;
    }
  }
  const banEnabledToggle = document.getElementById('banEnabledToggle');
  if (banEnabledToggle) {
    banEnabledToggle.onchange = () => setAutobanEnabled(!!banEnabledToggle.checked);
  }
  const banRefreshBtn = document.getElementById('banRefreshBtn');
  if (banRefreshBtn) banRefreshBtn.onclick = () => loadBans();
  const banUnbanFilterBtn = document.getElementById('banUnbanFilterBtn');
  if (banUnbanFilterBtn) banUnbanFilterBtn.onclick = () => unbanCurrentFilter();
  const banUnbanAllBtn = document.getElementById('banUnbanAllBtn');
  if (banUnbanAllBtn) banUnbanAllBtn.onclick = () => unbanAll();
  document.querySelectorAll('[data-ban-filter]').forEach((card) => {
    card.addEventListener('click', () => setBanFilter(card.getAttribute('data-ban-filter') || 'all'));
  });
    // ban page size select is rendered in banPager (bottom-left); restore prefs into banState.
  {
    const prefs = loadPrefs();
    if ([20,50,100].includes(Number(prefs.banPageSize))) {
      banState.pageSize = Number(prefs.banPageSize);
    }
  }
  // One-shot load on open; polling starts only when status reports running/applying.
  refresh();

  // 启动时若已自动读到 Key，直接加载自动禁用状态（无需再手填）
  syncKeyHint();
  updateAuthState();
  if (hasManagementKey()) {
    loadBans();
    loadSchedule();
  }

  (function bindLang() {
    const sel = document.getElementById('langSelect');
    if (sel) {
      sel.value = lang;
      sel.addEventListener('change', () => setLang(sel.value));
    }
    applyStaticI18n();
  })();
`
