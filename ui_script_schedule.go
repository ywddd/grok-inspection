package main

const uiScriptSchedule = `  const classLabel = {
    healthy: t('class_healthy'), permission_denied: t('class_permission_denied'), quota_exhausted: t('class_quota_exhausted'), spending_limit: t('class_spending_limit'),
    reauth: t('class_reauth'), model_unavailable: t('class_model_unavailable'), probe_error: t('class_probe_error'), unknown: t('class_unknown')
  };
  const actionLabel = { keep: t('action_keep'), disable: t('action_disable'), enable: t('action_enable'), delete: t('action_delete') };
  const color = {
    healthy: '#047857', permission_denied: '#b45309', quota_exhausted: '#b45309', spending_limit: '#c2410c',
    reauth: '#b91c1c', model_unavailable: '#475569', probe_error: '#b91c1c', unknown: '#475569'
  };
  function pill(text, c) {
    return '<span class="pill" style="background:' + c + '1a;color:' + c + '">' + escapeHtml(text) + '</span>';
  }
  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }
  const esc = escapeHtml;
  async function api(path, opts) {
    const headers = { 'Content-Type': 'application/json' };
    if (keyInput.value) headers.Authorization = 'Bearer ' + keyInput.value;
    const res = await fetch(BASE + path, Object.assign({ headers }, opts || {}));
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_) { data = { raw: text }; }
    // 202 Accepted is success for async apply/action
    if (!res.ok) throw new Error((data && (data.error || data.message)) || text || ('HTTP ' + res.status));
    return data;
  }
  let scheduleDirty = false;
  function scheduleStatusText(data) {
    if (!data || typeof data !== 'object') return t('schedule_loading');
    if (data.enabled === false) return t('schedule_disabled');
    const status = String(data.last_status || 'waiting');
    let text = status === 'running' ? t('schedule_running')
      : status === 'completed' ? t('schedule_completed')
      : status === 'failed' ? t('schedule_failed')
      : status === 'stopped' ? t('schedule_stopped')
      : status === 'action_failed' ? t('schedule_action_failed')
      : status === 'completed_with_errors' ? t('schedule_completed_errors')
      : status === 'skipped' ? t('schedule_skipped')
      : t('schedule_waiting');
    if (data.last_run_at) text += ' · ' + t('schedule_last') + formatShanghaiTime(data.last_run_at);
    if (data.next_run_at) text += ' · ' + t('schedule_next') + formatShanghaiTime(data.next_run_at);
    if (Number(data.last_matched || 0) > 0) {
      const matched403 = Number(data.last_matched_403 || 0);
      const matched402 = Number(data.last_matched_402 || 0);
      if (matched403 > 0) {
        text += ' · ' + t('schedule_counts') + matched403;
        text += t('schedule_disabled_count') + (data.last_disabled_403 || 0);
        text += t('schedule_deleted_count') + (data.last_deleted_403 || 0);
        text += t('schedule_failed_count') + (data.last_failed_403 || 0);
      }
      if (matched402 > 0) {
        text += ' · ' + t('schedule_counts_402') + matched402;
        text += t('schedule_disabled_count') + (data.last_disabled_402 || 0);
        text += t('schedule_deleted_count') + (data.last_deleted_402 || 0);
        text += t('schedule_failed_count') + (data.last_failed_402 || 0);
      }
      if (matched403 === 0 && matched402 === 0) {
        text += ' · ' + t('schedule_counts') + data.last_matched;
        text += t('schedule_disabled_count') + (data.last_disabled || 0);
        text += t('schedule_deleted_count') + (data.last_deleted || 0);
        text += t('schedule_failed_count') + (data.last_failed || 0);
      }
    }
    if (data.action_ready === false) text += ' · ' + t('schedule_key_missing');
    if (data.last_error) text += ' · ' + data.last_error;
    return text;
  }
  function renderSchedule(data, hydrate) {
    if (!data || typeof data !== 'object') return;
    if (hydrate && !scheduleDirty) {
      $('scheduleEnabled').checked = !!data.enabled;
      $('scheduleInterval').value = String(data.interval_minutes || 60);
      $('scheduleWorkers').value = String(clampWorkers(Number(data.workers) || WORKERS_DEFAULT));
      $('scheduleIncludeDisabled').checked = !!data.include_disabled;
      $('schedule403Action').value = data.permission_denied_action === 'delete' ? 'delete' : 'disable';
      $('schedule402Action').value = data.spending_limit_action === 'delete' ? 'delete' : 'disable';
    }
    const status = $('scheduleStatus');
    if (status) status.textContent = scheduleStatusText(data);
    const save = $('scheduleSaveBtn');
    if (save) save.disabled = !hasManagementKey();
  }
  async function loadSchedule() {
    if (!hasManagementKey()) {
      renderSchedule({ enabled: false, action_ready: false }, true);
      return;
    }
    try {
      const raw = await api('/schedule');
      const data = (raw && raw.result && typeof raw.result === 'object') ? raw.result : raw;
      renderSchedule(data, true);
    } catch (e) {
      const status = $('scheduleStatus');
      if (status) status.textContent = String(e.message || e);
    }
  }
  async function saveSchedule() {
    if (!hasManagementKey()) {
      showErr(t('need_key'));
      return;
    }
    const enabled = $('scheduleEnabled').checked;
    const interval = Number($('scheduleInterval').value);
    const workers = Number($('scheduleWorkers').value);
    if (!Number.isInteger(interval) || interval < SCHEDULE_INTERVAL_MIN || interval > SCHEDULE_INTERVAL_MAX) {
      showErr(t('schedule_interval') + ': ' + SCHEDULE_INTERVAL_MIN + '-' + SCHEDULE_INTERVAL_MAX);
      return;
    }
    if (!Number.isInteger(workers) || workers < WORKERS_MIN || workers > WORKERS_MAX) {
      showErr(t('workers_range_prefix') + WORKERS_MIN + '-' + WORKERS_MAX + t('workers_range_suffix'));
      return;
    }
    const action = $('schedule403Action').value === 'delete' ? 'delete' : 'disable';
    const action402 = $('schedule402Action').value === 'delete' ? 'delete' : 'disable';
    if (action === 'delete' || action402 === 'delete') {
      const only402Delete = action !== 'delete' && action402 === 'delete';
      const bothDelete = action === 'delete' && action402 === 'delete';
      const ok = await confirmDialog(
        bothDelete ? t('schedule_both_delete_confirm_title') : (only402Delete ? t('schedule_402_delete_confirm_title') : t('schedule_delete_confirm_title')),
        bothDelete ? t('schedule_both_delete_confirm_body') : (only402Delete ? t('schedule_402_delete_confirm_body') : t('schedule_delete_confirm_body'))
      );
      if (!ok) return;
    }
    const btn = $('scheduleSaveBtn');
    if (btn) btn.disabled = true;
    try {
      const raw = await api('/schedule', {
        method: 'POST',
        body: JSON.stringify({
          enabled,
          interval_minutes: interval,
          workers,
          include_disabled: $('scheduleIncludeDisabled').checked,
          permission_denied_action: action,
          spending_limit_action: action402
        })
      });
      const data = (raw && raw.result && typeof raw.result === 'object') ? raw.result : raw;
      scheduleDirty = false;
      renderSchedule(data, true);
      showOk(t('schedule_saved'));
    } catch (e) {
      showErr(String(e.message || e));
      if (btn) btn.disabled = false;
    }
  }
`
