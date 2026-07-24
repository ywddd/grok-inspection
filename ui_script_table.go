package main

const uiScriptTable = `  function filtered() {
    const rows = state.snapshot.results || [];
    if (state.filter === 'all') return rows;
    // 「异常」= 探测异常 / 模型不可用 / 未知 等非主分类
    if (state.filter === 'other') {
      return rows.filter((r) => {
        const c = r.classification || '';
        return c !== 'healthy' && c !== 'permission_denied' && c !== 'quota_exhausted' && c !== 'spending_limit' && c !== 'reauth';
      });
    }
    return rows.filter((r) => r.classification === state.filter);
  }
  function filterLabel() {
    const map = {
      all: t('class_all'),
      healthy: t('class_healthy'),
      permission_denied: t('class_permission_denied'),
      quota_exhausted: t('class_quota_exhausted'),
      spending_limit: t('class_spending_limit'),
      reauth: t('class_reauth'),
      other: t('class_other')
    };
    return map[state.filter] || state.filter;
  }
  function downloadBlob(filename, content, mime) {
    // UTF-8 BOM helps Windows Notepad open Chinese correctly.
    const blob = new Blob(['\uFEFF', content], { type: mime || 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  function decodeExportText(s) {
    return String(s == null ? '' : s)
      .replace(/&#34;/g, '"')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&amp;/g, '&');
  }
  function sanitizeExportRow(r) {
    const o = Object.assign({}, r || {});
    if (o.classification === 'healthy') {
      delete o.error_message;
      delete o.error_code;
    } else if (o.error_message) {
      let msg = decodeExportText(o.error_message);
      if (msg.length > 500) msg = msg.slice(0, 500) + '…';
      o.error_message = msg;
    }
    if (o.reason) o.reason = localizeKnownReason(decodeExportText(o.reason));
    return o;
  }
  function exportRows(format) {
    const rows = filtered().map(sanitizeExportRow);
    if (!rows.length) {
      showErr(t('no_export_filter'));
      return;
    }
    const stamp = new Date().toISOString().replace(/[:.]/g, '-');
    const tag = state.filter === 'all' ? 'all' : state.filter;
    if (format === 'json') {
      downloadBlob('grok-inspection-' + tag + '-' + stamp + '.json', JSON.stringify({
        filter: state.filter,
        filter_label: filterLabel(),
        exported_at: new Date().toISOString(),
        count: rows.length,
        results: rows
      }, null, 2), 'application/json;charset=utf-8');
      return;
    }
    const lines = [];
    lines.push('filter=' + filterLabel() + ' count=' + rows.length + ' exported_at=' + new Date().toISOString());
    lines.push(['name','disabled','classification','http_status','model','action','reason','auth_index','file_name','email'].join('\t'));
    rows.forEach((r) => {
      lines.push([
        r.name || '',
        r.disabled ? '1' : '0',
        r.classification || '',
        r.http_status || '',
        r.model || '',
        r.action || '',
        (r.reason || r.error_message || '').replace(/[\t\r\n]+/g, ' '),
        r.auth_index || '',
        r.file_name || '',
        r.email || ''
      ].join('\t'));
    });
    downloadBlob('grok-inspection-' + tag + '-' + stamp + '.txt', lines.join('\n'), 'text/plain;charset=utf-8');
  }
  function render() {
    const snap = state.snapshot || {};
    const summary = snap.summary || {};
    const cards = [
      ['total', t('class_all'), summary.total || 0],
      ['healthy', t('class_healthy'), summary.healthy || 0],
      ['permission_denied', t('class_permission_denied'), summary.permission_denied || 0],
      ['quota_exhausted', t('class_quota_exhausted'), summary.quota_exhausted || 0],
      ['spending_limit', t('class_spending_limit'), summary.spending_limit || 0],
      ['reauth', t('class_reauth'), summary.reauth || 0],
      ['other', t('class_other'), summary.other || 0],
    ];
    $('summary').innerHTML = cards.map(([key,label,value]) => {
      const active = (key === 'total' && state.filter === 'all') || state.filter === key;
      return '<div class="card' + (active ? ' active' : '') + '" data-filter="' + key + '"><div class="k">' + label + '</div><div class="v">' + value + '</div></div>';
    }).join('');
    $('summary').querySelectorAll('[data-filter]').forEach((el) => el.onclick = () => {
      state.filter = el.dataset.filter === 'total' ? 'all' : el.dataset.filter;
      state.page = 1; render();
    });

    const rows = filtered();
    $('exportHint').textContent = t('filter_count_prefix') + filterLabel() + t('filter_count_mid') + rows.length + t('filter_count_suffix');
    const totalPages = Math.max(1, Math.ceil(rows.length / state.pageSize));
    if (state.page > totalPages) state.page = totalPages;
    const start = (state.page - 1) * state.pageSize;
    const pageRows = rows.slice(start, start + state.pageSize);
    const tbody = $('rows');
    if (!pageRows.length) {
      tbody.innerHTML = '';
      $('empty').style.display = 'block';
      $('empty').textContent = hasManagementKey()
        ? t('click_start')
        : t('need_key_load');
    } else {
      $('empty').style.display = 'none';
      tbody.innerHTML = pageRows.map((r) => {
        const key = rowKey(r);
        const busy = pendingOps.has(key) || !!snap.applying;
        const toggleAct = r.disabled ? 'enable' : 'disable';
        const toggleLabel = r.disabled ? t('action_enable') : t('action_disable');
        // Every row always offers toggle + delete (not only classification-suggested action).
        const actionBtns = hasManagementKey()
          ? '<div class="row-actions">' +
              '<button type="button" data-act="' + toggleAct + '" ' + (busy ? 'disabled' : '') + '>' + toggleLabel + '</button>' +
              '<button type="button" class="danger" data-act="delete" ' + (busy ? 'disabled' : '') + '>' + t('action_delete') + '</button>' +
            '</div>'
          : '-';
        return '<tr data-key="' + escapeHtml(key) + '"' + (busy ? ' class="row-busy"' : '') + '>' +
          '<td class="col-name">' + escapeHtml(r.name) + '</td>' +
          '<td class="col-status">' + pill(r.disabled ? t('disabled') : t('enabled'), r.disabled ? '#b45309' : '#047857') + '</td>' +
          '<td class="col-result">' + pill(classLabel[r.classification] || r.classification || '-', color[r.classification] || '#475569') + '</td>' +
          '<td class="col-http">' + (r.http_status || '-') + '</td>' +
          '<td class="col-model">' + escapeHtml(r.model || '-') + '</td>' +
          '<td class="col-action">' + (actionLabel[r.action] || r.action || '-') + '</td>' +
          '<td class="col-reason">' + escapeHtml(localizeKnownReason(r.reason || r.error_message || '-') ) + '</td>' +
          '<td class="col-ops">' + actionBtns + '</td>' +
        '</tr>';
      }).join('');
      tbody.querySelectorAll('tr[data-key]').forEach((tr) => {
        const key = tr.getAttribute('data-key') || '';
        const r = pageRows.find((row) => rowKey(row) === key);
        if (!r) return;
        tr.querySelectorAll('button[data-act]').forEach((btn) => {
          btn.onclick = () => runRowAction(r, btn.dataset.act, tr);
        });
      });
    }
    const from = rows.length ? start + 1 : 0;
    const to = Math.min(rows.length, start + state.pageSize);
    $('pager').innerHTML =
      '<div class="pager-meta">' + t('showing') + from + '-' + to + ' / ' + rows.length +
      t('per_page') + '<select id="pageSize">' +
      [20,50,100].map((n) => '<option value="' + n + '"' + (state.pageSize===n?' selected':'') + '>' + n + '</option>').join('') +
      '</select></div>' +
      '<div style="display:flex;gap:8px;align-items:center">' +
      '<button id="prev"' + (state.page<=1?' disabled':'') + '>' + t('prev') + '</button>' +
      '<span class="pager-meta">' + state.page + ' / ' + totalPages + '</span>' +
      '<button id="next"' + (state.page>=totalPages?' disabled':'') + '>' + t('next') + '</button></div>';
    const ps = $('pageSize'); if (ps) ps.onchange = () => {
      state.pageSize = Number(ps.value)||20;
      savePrefs({ pageSize: state.pageSize });
      state.page=1;
      render();
    };
    const prev = $('prev'); if (prev) prev.onclick = () => { if (state.page>1){ state.page--; render(); } };
    const next = $('next'); if (next) next.onclick = () => { if (state.page<totalPages){ state.page++; render(); } };

    const actionCount = (snap.results || []).filter((r) => r.action === 'disable' || r.action === 'enable' || r.action === 'delete').length;
    const filteredCount = rows.length;
    const disableCount = rows.filter((r) => !r.disabled).length; // 当前分类下已启用 → 可禁用
    const enableCount = rows.filter((r) => !!r.disabled).length;  // 当前分类下已禁用 → 可启用
    const busy = !!(snap.running || snap.applying);
    const hasResults = (snap.results || []).length > 0;
    const filterCount = state.filter === 'all' ? 0 : filteredCount;
    $('runBtn').disabled = !hasManagementKey() || busy;
    $('incrBtn').disabled = !hasManagementKey() || busy || !hasResults;
    if ($('filterRunBtn')) {
      $('filterRunBtn').disabled = !hasManagementKey() || busy || state.filter === 'all' || filterCount === 0;
      $('filterRunBtn').textContent = filterCount ? (t('inspect_category') + ' (' + filterCount + ')') : t('inspect_category');
    }
    if ($('sampleBtn')) {
      $('sampleBtn').disabled = !hasManagementKey() || busy;
    }
    $('stopBtn').disabled = !hasManagementKey() || !(snap.running || snap.applying || (snap.unban && snap.unban.running));
    $('applyBtn').disabled = !hasManagementKey() || busy || actionCount === 0;
    $('batchExportBtn').disabled = filteredCount === 0;
    $('batchDisableBtn').disabled = !hasManagementKey() || busy || disableCount === 0;
    $('batchEnableBtn').disabled = !hasManagementKey() || busy || enableCount === 0;
    $('batchDeleteBtn').disabled = !hasManagementKey() || busy || filteredCount === 0;
    $('applyBtn').textContent = snap.applying
      ? (t('running') + ' ' + (snap.apply_done||0) + '/' + (snap.apply_total||0))
      : (actionCount ? (t('apply_suggested') + ' (' + actionCount + ')') : t('apply_suggested'));
    $('batchExportBtn').textContent = filteredCount ? (t('bulk_export') + ' (' + filteredCount + ')') : t('bulk_export');
    $('batchDisableBtn').textContent = disableCount ? (t('bulk_disable') + ' (' + disableCount + ')') : t('bulk_disable');
    $('batchEnableBtn').textContent = enableCount ? (t('bulk_enable') + ' (' + enableCount + ')') : t('bulk_enable');
    $('batchDeleteBtn').textContent = filteredCount ? (t('bulk_delete') + ' (' + filteredCount + ')') : t('bulk_delete');
    if (!hasManagementKey()) {
      setProgress(t('need_key_load'), false);
    } else if (snap.unban && snap.unban.running) {
      let msg = t('unban_running') + (snap.unban.done||0) + '/' + (snap.unban.total||0) + (snap.unban.current ? ' · ' + snap.unban.current : '');
      if ((snap.unban.failures || []).length) msg += t('unban_fail_sep') + snap.unban.failures.length; if (snap.unban.persist_error) msg += t('unban_persist_sep') + snap.unban.persist_error;
      setProgress(msg, true);
    } else if (snap.applying) {
      let msg = t('apply_progress') + (snap.apply_done||0) + '/' + (snap.apply_total||0) + (snap.apply_current ? t('apply_progress_sep') + snap.apply_current : '');
      if ((snap.apply_failures || []).length) msg += t('failed_sep') + snap.apply_failures.length;
      setProgress(msg, true);
    } else if (snap.running) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const sampling = !!snap.sample;
      const mode = sampling ? t('sample_running') : (scoped ? t('category_running') : (snap.incremental ? t('incremental_running') : t('inspect_running')));
      const extra = sampling ? t('sample_only_keep') : (scoped ? t('category_only_keep') : (snap.incremental ? t('incremental_only_keep') : t('bg_continue')));
      let phase = '';
      if (snap.probe_phase === 'retry') {
        phase = t('timeout_recheck') + (snap.retry_done||0) + '/' + (snap.retry_total||0) + t('recheck_workers') + (snap.retry_workers||1);
      }
      setProgress(mode + ' ' + (snap.done||0) + '/' + (snap.total||0) + t('workers_sep') + (snap.workers||WORKERS_DEFAULT) + phase + extra, true);
    } else if (snap.stopped) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const sampling = !!snap.sample;
      const mode = sampling ? t('sample_stopped') : (scoped ? t('category_stopped') : (snap.incremental ? t('incremental_stopped') : t('stopped')));
      setProgress(mode + t('this_run') + (snap.done||0) + (snap.total ? '/' + snap.total : '') + t('list_total') + ((snap.results||[]).length) + t('accounts_word'), false);
    } else if ((snap.results||[]).length) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const sampling = !!snap.sample;
      let msg = t('inspection_complete') + (snap.results||[]).length + t('accounts_word');
      if (sampling && (snap.done||0) >= 0 && snap.total != null) {
        msg = t('sample_complete') + (snap.done||0) + t('list_mid') + (snap.results||[]).length + t('list_end');
      } else if (scoped && (snap.done||0) >= 0 && snap.total != null) {
        msg = t('category_complete') + (snap.done||0) + t('list_mid') + (snap.results||[]).length + t('list_end');
      } else if (snap.incremental && (snap.done||0) >= 0 && snap.total != null) {
        msg = t('incremental_complete') + (snap.done||0) + t('list_mid') + (snap.results||[]).length + t('list_end');
      }
      if (snap.persist_error) msg += t('persist_fail_sep') + snap.persist_error; if (snap.store_path) msg += t('persisted');
      if ((snap.apply_failures || []).length) msg += t('last_fail') + snap.apply_failures.length + t('rows_word');
      setProgress(msg, false);
    } else {
      setProgress(t('waiting'), false);
    }
    const completedErrors = [];
    if ((snap.apply_failures || []).length && !snap.applying) {
      completedErrors.push(...(snap.apply_failures || []).map(localizeKnownActionError));
    }
    if (snap.unban && !snap.unban.running) {
      if ((snap.unban.failures || []).length) {
        completedErrors.push(...(snap.unban.failures || []).map(localizeKnownActionError));
      } else if (snap.unban.persist_error) {
        completedErrors.push(t('unban_progress_complete_fail') + localizeKnownActionError(snap.unban.persist_error));
      }
    }
    if (completedErrors.length) {
      // Keep failures visible after an asynchronous job has finished.
      showErr(completedErrors.join('\n'));
    }
  }
`
