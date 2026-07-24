package main

const uiScriptWire = `  function wireExclusive() {
    const include = $('includeDisabled');
    const only = $('onlyDisabled');
    $('workers').addEventListener('change', () => {
      try {
        const n = normalizeWorkersInput(true);
        savePrefs({ workers: n });
        if ($('error').className === 'err') $('error').textContent = '';
      } catch (e) {
        showErr(String(e.message || e));
        $('workers').value = String(WORKERS_DEFAULT);
        savePrefs({ workers: WORKERS_DEFAULT });
      }
    });
    include.onchange = () => {
      if (include.checked) only.checked = false;
      savePrefs({ includeDisabled: include.checked, onlyDisabled: only.checked });
    };
    only.onchange = () => {
      if (only.checked) include.checked = false;
      savePrefs({ includeDisabled: include.checked, onlyDisabled: only.checked });
    };
  }
  ['scheduleEnabled', 'scheduleInterval', 'scheduleWorkers', 'scheduleIncludeDisabled', 'schedule403Action', 'schedule402Action'].forEach((id) => {
    const el = $(id);
    if (el) el.addEventListener('input', () => { scheduleDirty = true; });
    if (el) el.addEventListener('change', () => { scheduleDirty = true; });
  });
  ['sampleCount', 'samplePercent'].forEach((id) => {
    const el = $(id);
    if (!el) return;
    ['input', 'change'].forEach((ev) => el.addEventListener(ev, () => {
      try { saveSamplePrefs(parseSampleInputs()); } catch (_) {}
    }));
  });
  const scheduleSaveBtn = $('scheduleSaveBtn');
  if (scheduleSaveBtn) scheduleSaveBtn.onclick = () => saveSchedule();
  $('runBtn').onclick = () => startInspection(false);
  $('incrBtn').onclick = () => startInspection(true);
  if ($('filterRunBtn')) $('filterRunBtn').onclick = () => startInspection('filter');
  if ($('sampleBtn')) $('sampleBtn').onclick = () => startInspection('sample');
  $('stopBtn').onclick = async () => {
    try { await api('/stop', { method: 'POST', body: JSON.stringify({ lang: lang }) }); await refresh(); }
    catch (e) { showErr(String(e.message || e)); }
  };
  $('applyBtn').onclick = async () => {
    const rows = state.snapshot.results || [];
    const disableN = rows.filter((r) => r.action === 'disable').length;
    const enableN = rows.filter((r) => r.action === 'enable').length;
    const deleteN = rows.filter((r) => r.action === 'delete').length;
    const actionCount = disableN + enableN + deleteN;
    const ok = await confirmDialog(
      t('apply_confirm_title'),
      t('apply_body_prefix') + actionCount + t('apply_body_suffix') +
      t('apply_disable_line') + disableN + t('apply_body_counts_mid') +
      t('apply_enable_line') + enableN + t('apply_body_counts_mid') +
      t('apply_delete_line') + deleteN + t('apply_body_counts_mid') + '\n' +
      t('continue_q')
    );
    if (!ok) return;
    try {
      const result = await api('/apply', { method: 'POST', body: JSON.stringify({ lang: lang }) });
      const total = Number(result && result.apply_total || 0);
      if (result && result.ok === false) throw new Error(result.error || t('start_failed'));
      showOk(total ? (t('suggested_running_prefix') + total + t('items_word')) : t('suggested_started'));
      await refresh();
    }
    catch (e) { showErr(String(e.message || e)); }
  };
  $('batchDisableBtn').onclick = () => batchForce('disable');
  $('batchEnableBtn').onclick = () => batchForce('enable');
  $('batchDeleteBtn').onclick = () => batchForce('delete');
  $('batchExportBtn').onclick = () => batchExport();
  wireExclusive();

`
