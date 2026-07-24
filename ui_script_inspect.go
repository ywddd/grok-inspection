package main

const uiScriptInspect = `  function parseSampleInputs() {
    const countRaw = String(($('sampleCount') && $('sampleCount').value) || '').trim();
    const percentRaw = String(($('samplePercent') && $('samplePercent').value) || '').trim();
    const count = countRaw === '' ? 0 : Number(countRaw);
    const percent = percentRaw === '' ? 0 : Number(percentRaw);
    if (!Number.isInteger(count) || count < 0) throw new Error(t('sample_invalid_params'));
    if (!Number.isInteger(percent) || percent < 0 || percent > 100) throw new Error(t('sample_invalid_params'));
    return { count, percent };
  }
  function saveSamplePrefs(sample) {
    savePrefs({
      sampleCount: sample.count ? sample.count : '',
      samplePercent: sample.percent ? sample.percent : ''
    });
  }
  async function startInspection(mode) {
    try {
      const workers = parseWorkersStrict();
      $('workers').value = String(workers);
      savePrefs({
        workers,
        includeDisabled: $('includeDisabled').checked,
        onlyDisabled: $('onlyDisabled').checked
      });
      const body = {
        workers,
        include_disabled: $('includeDisabled').checked,
        only_disabled: $('onlyDisabled').checked,
        incremental: false,
        sample: false,
        sample_count: 0,
        sample_percent: 0,
        classifications: [],
        lang: lang
      };
      if (mode === true) {
        body.incremental = true;
      } else if (mode === 'filter') {
        const classes = classificationsForFilter(state.filter);
        if (!classes.length) {
          showErr(t('pick_category_first'));
          return;
        }
        const count = filtered().length;
        if (!count) {
          showErr(t('no_cat_prefix') + filterLabel() + t('no_cat_suffix'));
          return;
        }
        body.classifications = classes;
      } else if (mode === 'sample') {
        const sample = parseSampleInputs();
        if (!sample.count && !sample.percent) {
          showErr(t('sample_need_params'));
          return;
        }
        saveSamplePrefs(sample);
        body.sample = true;
        body.sample_count = sample.count;
        body.sample_percent = sample.percent;
        // Sample follows the current category card when one is selected.
        if (state.filter !== 'all') {
          const classes = classificationsForFilter(state.filter);
          if (!classes.length) {
            showErr(t('pick_category_first'));
            return;
          }
          const count = filtered().length;
          if (!count) {
            showErr(t('no_cat_prefix') + filterLabel() + t('no_cat_suffix'));
            return;
          }
          body.classifications = classes;
        }
      }
      await api('/start', { method: 'POST', body: JSON.stringify(body)});
      await refresh();
    } catch (e) { showErr(String(e.message || e)); }
  }
  function rowKey(r) {
    return r.auth_index || r.file_name || r.name || r.email || '';
  }
  function actionTargetName(r) {
    // Prefer physical auth file name for CPA management delete/status APIs.
    return r.file_name || r.name || r.auth_index || r.email || '';
  }
  function sleep(ms) { return new Promise((resolve) => setTimeout(resolve, ms)); }
  // 202 Accepted ≠ success. Poll light /status for recent_row_actions[seq] (cheap, no full results).
  async function waitRowActionConfirmed(seq, key, act, timeoutMs) {
    const deadline = Date.now() + (timeoutMs || 30000);
    let lastErr = '';
    while (Date.now() < deadline) {
      const data = await api('/status?include_results=0&lang=' + encodeURIComponent(lang), { method: 'GET' });
      // Keep progress meta fresh while waiting.
      mergeLightStatus(data);
      const list = (data && data.recent_row_actions) || [];
      const hit = list.find((a) => Number(a && a.seq) === Number(seq));
      if (hit) {
        if (hit.ok) return { ok: true, report: hit };
        return { ok: false, error: localizeKnownActionError(hit.error || '') || (act + ' failed'), report: hit };
      }
      lastErr = t('still_running');
      render(); // show row-busy / running
      await sleep(200);
    }
    return { ok: false, error: lastErr || t('action_timeout') };
  }
  async function runRowAction(r, act, tr) {
    const key = rowKey(r);
    if (!key || pendingOps.has(key)) return;
    if (!hasManagementKey()) {
      showErr(t('need_key'));
      return;
    }
    const label = act === 'delete' ? t('action_delete') : (act === 'enable' ? t('action_enable') : t('action_disable'));
    if (act === 'delete') {
      const ok = await confirmDialog(t('delete_confirm_title'), t('delete_body_prefix') + (r.name || key) + t('delete_body_suffix'));
      if (!ok) return;
    }
    pendingOps.add(key);
    if (tr) tr.classList.add('row-busy');
    showOk(t('running_prefix') + (r.name || key));
    render();
    try {
      const result = await api('/action', {
        method: 'POST',
        body: JSON.stringify({
          auth_index: r.auth_index || '',
          name: actionTargetName(r),
          disabled: act === 'disable',
          delete: act === 'delete',
          lang: lang
        })
      });
      if (!result || result.ok === false) {
        throw new Error((result && result.error) || (label + t('action_failed_suffix')));
      }
      const seq = Number(result.action_seq || 0);
      if (!seq) {
        throw new Error(t('no_action_seq'));
      }
      // Wait for server completion via light status (not optimistic success).
      const confirmed = await waitRowActionConfirmed(seq, key, act, 30000);
      if (!confirmed.ok) {
        throw new Error(confirmed.error || (label + t('action_failed_suffix')));
      }
      // Confirmed success → pull full results once, then UI feedback.
      await refresh({ light: false });
      if (act === 'delete') {
        // Full refresh already dropped the row; optional fade if still painted.
        const rowEl = Array.from(document.querySelectorAll('tr[data-key]')).find((el) => el.getAttribute('data-key') === key);
        if (rowEl) {
          rowEl.classList.add('row-out');
          await sleep(180);
          render();
        }
        showOk(t('deleted_prefix') + (r.name || key));
      } else {
        showOk((act === 'disable' ? t('disabled_prefix') : t('enabled_prefix')) + (r.name || key));
      }
    } catch (e) {
      showErr(String(e.message || e));
      await refresh({ light: false });
    } finally {
      pendingOps.delete(key);
      render();
    }
  }
  // 批量禁用：只针对当前分类下「已启用」的号；批量启用：只针对「已禁用」的号。
  function filteredRowsForAction(action) {
    const rows = filtered();
    if (action === 'disable') return rows.filter((r) => !r.disabled);
    if (action === 'enable') return rows.filter((r) => !!r.disabled);
    return rows; // delete / export 用全部分类内账号
  }
  function filteredAuthIndexesForAction(action) {
    return filteredRowsForAction(action).map(rowKey).filter(Boolean);
  }
  async function batchForce(action) {
    const targetRows = filteredRowsForAction(action);
    const indexes = targetRows.map(rowKey).filter(Boolean);
    if (!targetRows.length || !indexes.length) {
      const tip = action === 'disable'
        ? t('no_enabled')
        : (action === 'enable' ? t('no_disabled') : t('no_actionable'));
      showErr(t('no_cat_prefix') + filterLabel() + t('no_cat_action_mid') + tip);
      return;
    }
    const label = action === 'delete' ? t('action_delete') : (action === 'enable' ? t('action_enable') : t('action_disable'));
    const stateHint = action === 'disable'
      ? t('only_enabled_rows')
      : (action === 'enable' ? t('only_disabled_rows') : t('all_in_category'));
    let extra = '';
    if (action === 'delete') {
      extra = t('bulk_delete_api') + t('irreversible');
    } else {
      extra = t('bulk_patch_prefix') + label + t('bulk_patch_suffix') + t('bulk_patch_note') + t('bulk_patch_slow') + t('bulk_patch_hint');
    }
    const ok = await confirmDialog(
      t('bulk_confirm_title_prefix') + label + t('bulk_confirm_title_suffix'),
      t('current_category') + filterLabel() + '\n' +
      t('affected') + indexes.length + t('units') +
      stateHint + '\n\n' +
      t('will_bulk') + label + '.\n' + extra + '\n\n' +
      t('continue_q')
    );
    if (!ok) return;
    try {
      const result = await api('/apply', {
        method: 'POST',
        body: JSON.stringify({
          force_action: action,
          auth_indexes: indexes,
          lang: lang
        })
      });
      const total = Number(result && result.apply_total || indexes.length || 0);
      showOk(t('bulk_started_prefix') + label + t('started_prefix') + total + t('started_suffix'));
      await refresh();
    } catch (e) {
      showErr(String(e.message || e));
    }
  }
  async function batchExport() {
    const rows = filtered();
    if (!rows.length) {
      showErr(t('no_cat_prefix') + filterLabel() + t('no_export_suffix'));
      return;
    }
    const ok = await confirmDialog(
      t('export_confirm'),
      t('current_category') + filterLabel() + '\n' +
      t('export_count') + rows.length + t('export_rows') +
      t('export_body') +
      t('continue_q')
    );
    if (!ok) return;
    exportRows('json');
  }
  // Persist key on input (not only blur/change) so paste + click 开始 doesn't lose it next visit.
  let keySaveTimer = null;
  keyInput.addEventListener('input', () => {
    keySource = 'manual';
    updateAuthState();
    if (keySaveTimer) clearTimeout(keySaveTimer);
    keySaveTimer = setTimeout(() => persistManagementKey(keyInput.value), 200);
  });
  keyInput.addEventListener('change', () => {
    keySource = 'manual';
    persistManagementKey(keyInput.value);
    updateAuthState();
    refresh();
    loadSchedule();
  });
  // If a key is available, keep plugin storage in sync for next open.
  if (keyInput.value.trim()) persistManagementKey(keyInput.value);
  updateAuthState();
`
