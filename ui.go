package main

import "fmt"

func renderUIPage(pluginID string) []byte {
	base := "/v0/management/plugins/" + pluginID
	html := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Grok 账号巡检</title>
  <style>
    :root { color-scheme: light; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:#f5f7fb; color:#0f172a; }
    .wrap { max-width: 1480px; margin: 0 auto; padding: 18px clamp(12px,2vw,24px) 28px; }
    .hero { display:flex; justify-content:space-between; gap:16px; flex-wrap:wrap; margin-bottom:14px; }
    .badge { display:inline-flex; align-items:center; height:22px; padding:0 8px; border-radius:999px; background:#eef2ff; color:#3730a3; font-size:11px; font-weight:700; }
    h1 { margin:6px 0 0; font-size:22px; line-height:30px; }
    .sub { margin:4px 0 0; color:#64748b; font-size:13px; }
    .controls { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    label.ctl, button { height:34px; border-radius:8px; font-size:13px; }
    label.ctl { display:inline-flex; align-items:center; gap:6px; padding:0 10px; border:1px solid #dbe1e8; background:#fff; color:#475569; }
    input[type=number] { width:56px; height:26px; border:1px solid #cbd5e1; border-radius:6px; padding:0 6px; }
    button { padding:0 12px; border:1px solid #d1d5db; background:#fff; color:#334155; cursor:pointer; }
    button.primary { border-color:#2563eb; background:#2563eb; color:#fff; font-weight:700; }
    button.soft { border-color:#c7d2fe; background:#eef2ff; color:#3730a3; font-weight:650; }
    button.danger { border-color:#fecaca; background:#fef2f2; color:#b91c1c; font-weight:650; }
    button:disabled { opacity:.55; cursor:not-allowed; }
    .summary { display:grid; grid-template-columns:repeat(6,minmax(100px,1fr)); gap:10px; margin-bottom:12px; }
    .card { background:#fff; border:1px solid #e2e8f0; border-radius:10px; padding:12px; box-shadow:0 1px 2px rgba(15,23,42,.04); cursor:pointer; }
    .card.active { outline:2px solid #2563eb; }
    .card .k { color:#64748b; font-size:12px; }
    .card .v { margin-top:4px; font-size:22px; font-weight:750; }
    .bar { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:10px; align-items:center; }
    .actions-row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    .actions-row .hint { font-size:12px; color:#64748b; }
    .progress { min-height:20px; font-size:12px; color:#64748b; display:inline-flex; align-items:center; gap:6px; padding:4px 10px; border-radius:8px; max-width:100%%; }
    .progress.live { color:#1d4ed8; font-weight:700; background:#dbeafe; border:1px solid #93c5fd; box-shadow:0 0 0 1px rgba(37,99,235,.08); }
    .progress.live::before { content:""; width:8px; height:8px; border-radius:50%%; background:#2563eb; box-shadow:0 0 0 0 rgba(37,99,235,.55); animation:pulseDot 1.2s ease-out infinite; flex:0 0 auto; }
    @keyframes pulseDot { 0%% { box-shadow:0 0 0 0 rgba(37,99,235,.45); } 70%% { box-shadow:0 0 0 8px rgba(37,99,235,0); } 100%% { box-shadow:0 0 0 0 rgba(37,99,235,0); } }
    tr.row-out { opacity:0; transform:translateX(8px); transition:opacity .28s ease, transform .28s ease; }
    tr.row-busy { opacity:.55; }
    .row-actions { display:flex; gap:6px; flex-wrap:wrap; align-items:center; }
    .row-actions button { height:28px; padding:0 8px; font-size:12px; }
    .toast-ok { color:#047857; font-size:12px; margin-top:6px; }
    .modal { position:fixed; inset:0; z-index:1000; display:flex; align-items:center; justify-content:center; background:rgba(15,23,42,.45); padding:16px; }
    .modal.hidden { display:none; }
    .modal-card { width:min(440px,100%%); background:#fff; border-radius:12px; border:1px solid #e2e8f0; box-shadow:0 20px 40px rgba(15,23,42,.18); padding:18px 18px 14px; }
    .modal-title { font-size:16px; font-weight:700; color:#0f172a; margin-bottom:10px; }
    .modal-msg { font-size:13px; line-height:1.6; color:#334155; white-space:pre-wrap; margin-bottom:16px; }
    .modal-actions { display:flex; justify-content:flex-end; gap:8px; }
    .modal-actions button { min-width:76px; }
    .table-wrap { background:#fff; border:1px solid #e2e8f0; border-radius:10px; overflow:hidden; box-shadow:0 1px 2px rgba(15,23,42,.04); }
    table { width:100%%; border-collapse:collapse; min-width:1100px; font-size:13px; table-layout:auto; }
    th { padding:10px 12px; border-bottom:1px solid #e2e8f0; text-align:left; background:linear-gradient(180deg,#f8fafc 0%%,#f1f5f9 100%%); color:#475569; font-size:12px; white-space:nowrap; }
    td { padding:10px 12px; border-bottom:1px solid #f1f5f9; vertical-align:middle; }
    td.col-reason { vertical-align:top; word-break:break-word; overflow-wrap:anywhere; }
    th.col-status, td.col-status, th.col-result, td.col-result { white-space:nowrap; width:1%%; min-width:88px; }
    th.col-http, td.col-http { white-space:nowrap; width:1%%; min-width:56px; text-align:center; }
    th.col-model, td.col-model { white-space:nowrap; min-width:72px; }
    th.col-action, td.col-action { white-space:nowrap; min-width:72px; }
    th.col-ops, td.col-ops { white-space:nowrap; width:1%%; min-width:120px; }
    .pill { display:inline-flex; align-items:center; height:22px; padding:0 10px; border-radius:999px; font-size:12px; font-weight:650; white-space:nowrap; flex-shrink:0; line-height:22px; max-width:none; }
    .empty { padding:48px 20px; text-align:center; color:#64748b; }
    .pager { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; padding:10px 12px; border-top:1px solid #e2e8f0; background:#fbfdff; align-items:center; }
    .err { color:#b91c1c; white-space:pre-wrap; }
    .key-row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; width:100%%; }
    .key-row input { width:min(360px,100%%); height:34px; border:1px solid #cbd5e1; border-radius:8px; padding:0 10px; }
    :root {
      color-scheme: light;
      --page-bg: #f5f7fb;
      --surface: #ffffff;
      --surface-muted: #fbfdff;
      --surface-subtle: #f8fafc;
      --text: #0f172a;
      --muted: #64748b;
      --border: #e2e8f0;
      --border-subtle: #f1f5f9;
      --input-border: #cbd5e1;
    }
    html, body { min-width:0; background:var(--page-bg) !important; color:var(--text) !important; }
    .grok-inspection-page { min-width:0; color:var(--text) !important; }
    .grok-inspection-page .sub,
    .grok-inspection-page .actions-row .hint,
    .grok-inspection-page .card .k { color:var(--muted) !important; }
    .grok-inspection-page .progress { color:var(--muted) !important; }
    .grok-inspection-page .progress.live { color:#1d4ed8 !important; background:#dbeafe !important; border-color:#93c5fd !important; }
    .grok-inspection-page .ctl,
    .grok-inspection-page button,
    .grok-inspection-page .card,
    .grok-inspection-page .table-wrap,
    .grok-inspection-page .modal-card { color:var(--text) !important; background:var(--surface) !important; border-color:var(--border) !important; }
    .grok-inspection-page button.primary { background:#2563eb !important; border-color:#2563eb !important; color:#fff !important; }
    .grok-inspection-page button.soft { background:#eef2ff !important; border-color:#c7d2fe !important; color:#3730a3 !important; }
    .grok-inspection-page button.danger { background:#fef2f2 !important; border-color:#fecaca !important; color:#b91c1c !important; }
    .grok-inspection-page .modal-msg { color:var(--text) !important; }
    .grok-inspection-page input[type=number],
    .grok-inspection-page .key-row input { color:var(--text) !important; background:var(--surface) !important; border-color:var(--input-border) !important; }
    .grok-inspection-page th { background:var(--surface-subtle) !important; color:var(--muted) !important; border-color:var(--border) !important; }
    .grok-inspection-page td { border-color:var(--border-subtle) !important; }
    .grok-inspection-page .pager { background:var(--surface-muted) !important; border-color:var(--border) !important; }
    .grok-inspection-page .empty { color:var(--muted) !important; }
    .grok-inspection-page .settings-row,
    .grok-inspection-page .actions-row { display:flex; gap:8px; flex-wrap:wrap; width:100%%; }
    .grok-inspection-page .settings-row > .ctl,
    .grok-inspection-page .actions-row > button { min-width:0; }
    html[data-grok-theme="dark"] {
      color-scheme: dark;
      --page-bg: #111827;
      --surface: #182131;
      --surface-muted: #151d2b;
      --surface-subtle: #1d2737;
      --text: #f8fafc;
      --muted: #a7b3c7;
      --border: #334155;
      --border-subtle: #273449;
      --input-border: #475569;
    }
    html[data-grok-theme="dark"] .grok-inspection-page button.soft { background:#242c58 !important; border-color:#4b5aa6 !important; color:#dbe4ff !important; }
    html[data-grok-theme="dark"] .grok-inspection-page button.danger { background:#3f1d1d !important; border-color:#7f1d1d !important; color:#fecaca !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .badge { background:#252b63 !important; color:#c7d2fe !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .card.active { outline-color:#60a5fa !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .progress.live { color:#93c5fd !important; background:#1e3a5f !important; border-color:#3b82f6 !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .progress.live::before { background:#60a5fa; }
    @media (max-width:760px){
      body { overflow-x:hidden !important; }
      .grok-inspection-page { padding:14px 12px calc(24px + env(safe-area-inset-bottom)); }
      .grok-inspection-page .hero { display:block; }
      .grok-inspection-page h1 { font-size:24px; line-height:30px; }
      .grok-inspection-page .controls { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; width:100%%; }
      .grok-inspection-page .key-row { grid-column:1 / -1; grid-row:1; width:100%%; }
      .grok-inspection-page .key-row input { width:100%%; min-width:0; height:42px; font-size:16px; }
      .grok-inspection-page .controls > label { width:100%%; min-width:0; padding:0 8px; }
      .grok-inspection-page .controls > label:first-of-type { grid-column:1 / -1; grid-row:2; }
      .grok-inspection-page .controls > label:nth-of-type(2) { grid-column:1; grid-row:3; }
      .grok-inspection-page .controls > label:nth-of-type(3) { grid-column:2; grid-row:3; }
      .grok-inspection-page input[type=number] { flex:1; width:100%%; min-width:0; }
      .grok-inspection-page .controls > #stopBtn { grid-column:1; grid-row:4; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #runBtn { grid-column:2; grid-row:4; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #incrBtn { grid-column:1 / -1; grid-row:5; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #applyBtn { grid-column:1 / -1; grid-row:6; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .summary { grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; }
      .grok-inspection-page .card { min-width:0; padding:10px; }
      .grok-inspection-page .card .v { font-size:26px; }
      .grok-inspection-page .bar { display:block; }
      .grok-inspection-page .actions-row {
        display:grid; grid-template-columns:repeat(2,minmax(0,1fr));
        margin-top:8px; width:100%%; gap:8px;
      }
      .grok-inspection-page .actions-row > button { width:100%%; min-width:0; padding:0 8px; }
      .grok-inspection-page .actions-row .hint {
        grid-column:1 / -1; line-height:1.5; overflow-wrap:anywhere;
      }
      .grok-inspection-page .pager { align-items:stretch; }
      .grok-inspection-page .pager > div { width:100%%; }
      .grok-inspection-page .pager > div:last-child { justify-content:space-between; }
    }
  </style>
</head>
<body>
  <div class="wrap grok-inspection-page">
    <div class="hero">
      <div>
        <div class="badge">xAI / Grok · CPA Plugin</div>
        <h1>Grok 账号巡检</h1>
        <p class="sub">「开始巡检」清空并重测全部；「增量巡检」只测新增账号；「巡检当前分类」只重测所选分类；「批量操作」作用于当前筛选；可开启定时自动巡检+自动处置；结果会自动保存。</p>
      </div>
      <div class="controls">
        <div class="key-row" id="keyRow">
          <input id="managementKey" type="password" autocomplete="current-password" placeholder="CPA Management Key（可自动读取管理面板）">
          <span class="hint" id="keyHint" style="font-size:12px;color:#64748b"></span>
        </div>
        <label class="ctl">并发 <input id="workers" type="number" min="1" max="16" step="1" value="6" title="1-16 的整数"></label>
        <label class="ctl"><input id="includeDisabled" type="checkbox"> 包含已禁用</label>
        <label class="ctl"><input id="onlyDisabled" type="checkbox"> 仅巡检已禁用</label>
        <button id="stopBtn" disabled>停止</button>
        <button id="applyBtn" class="soft" disabled>执行建议操作</button>
        <button id="incrBtn" class="soft" disabled title="只检测 Auth 中相对上次结果新增的账号">增量巡检</button>
        <button id="filterRunBtn" class="soft" disabled title="只重新探测当前卡片筛选分类下的账号，保留其他结果">巡检当前分类</button>
        <button id="runBtn" class="primary">开始巡检</button>
      </div>
    </div>
    <div class="schedule-panel" id="schedulePanel" style="background:#fff;border:1px solid #e2e8f0;border-radius:10px;padding:12px 14px;margin-bottom:12px;box-shadow:0 1px 2px rgba(15,23,42,.04)">
      <div style="display:flex;flex-wrap:wrap;gap:10px 16px;align-items:center;justify-content:space-between">
        <div>
          <div style="font-weight:700;font-size:14px">定时自动巡检</div>
          <div class="sub" style="margin:2px 0 0;font-size:12px">默认每 10 分钟全量巡检（含已禁用）；结束后自动：权限被拒→删除 · 额度用尽→禁用 · 健康且禁用→启用 · 健康且启用→不动</div>
        </div>
        <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center">
          <label class="ctl"><input id="schedEnabled" type="checkbox"> 启用定时</label>
          <label class="ctl">间隔(分) <input id="schedInterval" type="number" min="1" max="1440" step="1" value="10" style="width:64px"></label>
          <label class="ctl"><input id="schedAutoApply" type="checkbox" checked> 巡检后自动处置</label>
          <button id="schedSaveBtn" class="soft" type="button" disabled>保存定时</button>
        </div>
      </div>
      <div id="schedStatus" class="hint" style="margin-top:8px;font-size:12px;color:#64748b">定时未加载</div>
    </div>
    <div id="summary" class="summary"></div>
    <div class="bar">
      <div class="actions-row">
        <button id="batchExportBtn" type="button" disabled>批量导出</button>
        <button id="batchDisableBtn" class="soft" type="button" disabled>批量禁用</button>
        <button id="batchEnableBtn" class="soft" type="button" disabled>批量启用</button>
        <button id="batchDeleteBtn" class="danger" type="button" disabled>批量删除</button>
        <span class="hint" id="exportHint">点击上方卡片切换分类；禁用/启用数量按当前分类下列表的启用/禁用状态统计</span>
      </div>
      <div style="display:flex;flex-direction:column;align-items:flex-end;gap:4px;min-width:0;max-width:100%%">
        <div id="progress" class="progress">等待开始</div>
        <pre id="error" class="err" style="margin:0;max-width:min(720px,100%%);text-align:left;font-size:12px;line-height:1.45;white-space:pre-wrap;word-break:break-word"></pre>
      </div>
    </div>
    <div id="confirmModal" class="modal hidden" aria-hidden="true">
      <div class="modal-card" role="dialog" aria-modal="true">
        <div id="confirmTitle" class="modal-title">确认操作</div>
        <div id="confirmMsg" class="modal-msg"></div>
        <div class="modal-actions">
          <button type="button" id="confirmCancel">取消</button>
          <button type="button" id="confirmOk" class="primary">确定</button>
        </div>
      </div>
    </div>
    <div class="table-wrap">
      <div style="overflow:auto">
        <table>
          <thead>
            <tr>
              <th>账号</th><th class="col-status">当前状态</th><th class="col-result">检测结果</th><th class="col-http">HTTP</th><th class="col-model">模型</th><th class="col-action">建议</th><th class="col-reason">原因</th><th class="col-ops">操作</th>
            </tr>
          </thead>
          <tbody id="rows"></tbody>
        </table>
      </div>
      <div id="empty" class="empty">请输入 CPA Management Key 后加载巡检状态</div>
      <div id="pager" class="pager"></div>
    </div>
  </div>
  <script>
  function namedTheme(value) {
    const text = String(value || '').trim().toLowerCase();
    if (text.includes('dark') || text.includes('night')) return 'dark';
    if (text.includes('light') || text.includes('day')) return 'light';
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
  function detectHostTheme() {
    try {
      if (window.parent && window.parent !== window) {
        const doc = window.parent.document;
        return elementTheme(doc.documentElement) || elementTheme(doc.body) || backgroundTheme(doc);
      }
    } catch (_) {}
    return elementTheme(document.documentElement) || elementTheme(document.body) || backgroundTheme(document) || 'light';
  }
  function syncHostTheme() {
    document.documentElement.setAttribute('data-grok-theme', detectHostTheme() || 'light');
  }
  syncHostTheme();
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
  window.addEventListener('pageshow', syncHostTheme);
  window.addEventListener('storage', syncHostTheme);

  const BASE = %q;
  const WORKERS_MIN = 1;
  const WORKERS_MAX = 16;
  const WORKERS_DEFAULT = 6;
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
      throw new Error('并发必须是 ' + WORKERS_MIN + '-' + WORKERS_MAX + ' 的整数（当前默认 ' + WORKERS_DEFAULT + '）');
    }
    const n = Number(raw);
    if (!Number.isInteger(n) || n < WORKERS_MIN || n > WORKERS_MAX) {
      throw new Error('并发必须在 ' + WORKERS_MIN + '-' + WORKERS_MAX + ' 之间');
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
      for (let i = 0; i < encrypted.length; i++) out[i] = encrypted[i] ^ key[i %% key.length];
      return new TextDecoder().decode(out);
    } catch (_) { return raw; }
  }
  function tryParseJSON(text) {
    try { return JSON.parse(text); } catch (_) { return null; }
  }
  function extractKeyFromPanelStorage() {
    try {
      // Zustand persist key used by Management Center auth store.
      const authRaw = localStorage.getItem('cli-proxy-auth');
      if (authRaw) {
        const parsed = tryParseJSON(deobfuscatePanelValue(authRaw));
        const key = (parsed && parsed.state && parsed.state.managementKey)
          || (parsed && parsed.managementKey)
          || '';
        if (String(key).trim()) return String(key).trim();
      }
      // Legacy plaintext / obfuscated single-key entries.
      for (const name of ['managementKey', 'cli-proxy-management-key']) {
        const raw = localStorage.getItem(name);
        if (!raw) continue;
        const plain = deobfuscatePanelValue(raw);
        const parsed = tryParseJSON(plain);
        if (typeof parsed === 'string' && parsed.trim()) return parsed.trim();
        if (parsed && typeof parsed === 'object' && parsed.managementKey) {
          return String(parsed.managementKey).trim();
        }
        if (plain && plain.indexOf(PANEL_ENC_PREFIX) !== 0 && plain.trim()) return plain.trim();
      }
    } catch (_) {}
    return '';
  }
  function loadStoredManagementKey() {
    try {
      const own = localStorage.getItem(KEY_STORAGE) || sessionStorage.getItem(KEY_STORAGE) || '';
      if (own && String(own).trim()) return String(own).trim();
    } catch (_) {}
    return extractKeyFromPanelStorage();
  }
  function persistManagementKey(value) {
    const v = String(value == null ? '' : value);
    try {
      localStorage.setItem(KEY_STORAGE, v);
      sessionStorage.setItem(KEY_STORAGE, v);
    } catch (_) {}
  }
  let keySource = 'manual'; // panel | plugin | manual
  const bootKey = loadStoredManagementKey();
  keyInput.value = bootKey;
  if (bootKey) {
    if (extractKeyFromPanelStorage() === bootKey) keySource = 'panel';
    else keySource = 'plugin';
  }
  function syncKeyHint() {
    const hint = $('keyHint');
    if (!hint) return;
    if (hasManagementKey() && keySource === 'panel') {
      hint.textContent = '已从管理面板自动读取 Key（无需手填）';
      keyInput.placeholder = '已自动填充（可改）';
    } else if (hasManagementKey() && keySource === 'plugin') {
      hint.textContent = '已使用本插件本地保存的 Key';
    } else {
      hint.textContent = '未读到 Key：请先登录 /management.html 并勾选记住密码，或在此手动填写';
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
  function closeConfirm(ok) {
    $('confirmModal').classList.add('hidden');
    $('confirmModal').setAttribute('aria-hidden', 'true');
    const resolve = confirmResolver;
    confirmResolver = null;
    if (resolve) resolve(!!ok);
  }
  function confirmDialog(title, message) {
    return new Promise((resolve) => {
      confirmResolver = resolve;
      $('confirmTitle').textContent = title || '确认操作';
      $('confirmMsg').textContent = message || '';
      $('confirmModal').classList.remove('hidden');
      $('confirmModal').setAttribute('aria-hidden', 'false');
      $('confirmOk').focus();
    });
  }
  function classificationsForFilter(filter) {
    if (!filter || filter === 'all') return [];
    // Server expands "other" to non-primary classes.
    return [filter];
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
        classifications: []
      };
      if (mode === true) {
        body.incremental = true;
      } else if (mode === 'filter') {
        const classes = classificationsForFilter(state.filter);
        if (!classes.length) {
          showErr('请先点击分类卡片选择一个分类，再巡检当前分类');
          return;
        }
        const count = filtered().length;
        if (!count) {
          showErr('当前分类「' + filterLabel() + '」下没有可巡检账号');
          return;
        }
        body.classifications = classes;
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
      const data = await api('/status?include_results=0', { method: 'GET' });
      // Keep progress meta fresh while waiting.
      mergeLightStatus(data);
      const list = (data && data.recent_row_actions) || [];
      const hit = list.find((a) => Number(a && a.seq) === Number(seq));
      if (hit) {
        if (hit.ok) return { ok: true, report: hit };
        return { ok: false, error: hit.error || (act + ' failed'), report: hit };
      }
      lastErr = '仍在执行…';
      render(); // show row-busy / 执行中
      await sleep(200);
    }
    return { ok: false, error: lastErr || '操作超时，请刷新后确认是否已生效' };
  }
  async function runRowAction(r, act, tr) {
    const key = rowKey(r);
    if (!key || pendingOps.has(key)) return;
    if (!hasManagementKey()) {
      showErr('请先填写 CPA Management Key');
      return;
    }
    const label = act === 'delete' ? '删除' : (act === 'enable' ? '启用' : '禁用');
    if (act === 'delete') {
      const ok = await confirmDialog('删除确认', '将删除 CPA Auth 凭证「' + (r.name || key) + '」。\n此操作不可恢复，确认继续？');
      if (!ok) return;
    }
    pendingOps.add(key);
    if (tr) tr.classList.add('row-busy');
    showOk(label + '执行中：' + (r.name || key));
    render();
    try {
      const result = await api('/action', {
        method: 'POST',
        body: JSON.stringify({
          auth_index: r.auth_index || '',
          name: actionTargetName(r),
          disabled: act === 'disable',
          delete: act === 'delete'
        })
      });
      if (!result || result.ok === false) {
        throw new Error((result && result.error) || (label + '失败'));
      }
      const seq = Number(result.action_seq || 0);
      if (!seq) {
        throw new Error('服务端未返回 action_seq，无法确认执行结果');
      }
      // Wait for server completion via light status (not optimistic success).
      const confirmed = await waitRowActionConfirmed(seq, key, act, 30000);
      if (!confirmed.ok) {
        throw new Error(confirmed.error || (label + '失败'));
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
        showOk('删除成功：' + (r.name || key));
      } else {
        showOk((act === 'disable' ? '禁用成功：' : '启用成功：') + (r.name || key));
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
        ? '没有「已启用」可禁用的账号'
        : (action === 'enable' ? '没有「已禁用」可启用的账号' : '没有可操作的账号');
      showErr('当前分类「' + filterLabel() + '」下' + tip);
      return;
    }
    const label = action === 'delete' ? '删除' : (action === 'enable' ? '启用' : '禁用');
    const stateHint = action === 'disable'
      ? '仅包含当前列表中状态为「已启用」的账号。'
      : (action === 'enable' ? '仅包含当前列表中状态为「已禁用」的账号。' : '包含当前分类下全部账号。');
    let extra = '';
    if (action === 'delete') {
      extra =
        '将调用 CPA 本体批量删除接口（DELETE /auth-files，每批最多 50 个），并更新本地结果。\n' +
        '此操作不可恢复。';
    } else {
      extra =
        '将通过 CPA Management API ' + label + '账号，并更新本地结果。\n' +
        '说明：CPA 本体没有批量启用/禁用接口，只能逐个调用 PATCH（插件侧会并发约 6 路），' +
        '账号多时可能较慢，上方会显示进度。\n' +
        '若需要更快清理，可改用「批量删除」（本体支持一次删多个）。';
    }
    const ok = await confirmDialog(
      '批量' + label + '确认',
      '当前分类：' + filterLabel() + '\n' +
      '影响账号：' + indexes.length + ' 个\n' +
      stateHint + '\n\n' +
      '将对上述账号执行批量' + label + '。\n' + extra + '\n\n' +
      '请确认是否继续？'
    );
    if (!ok) return;
    try {
      const result = await api('/apply', {
        method: 'POST',
        body: JSON.stringify({
          force_action: action,
          auth_indexes: indexes
        })
      });
      const total = Number(result && result.apply_total || indexes.length || 0);
      showOk('批量' + label + '已启动：共 ' + total + ' 项（后台执行，进度见上方状态）');
      await refresh();
    } catch (e) {
      showErr(String(e.message || e));
    }
  }
  async function batchExport() {
    const rows = filtered();
    if (!rows.length) {
      showErr('当前分类「' + filterLabel() + '」下没有可导出的数据');
      return;
    }
    const ok = await confirmDialog(
      '批量导出确认',
      '当前分类：' + filterLabel() + '\n' +
      '导出条数：' + rows.length + ' 条\n\n' +
      '将导出当前分类下的全部账号（不是仅当前页）为 JSON 文件。\n\n' +
      '请确认是否继续？'
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
  });
  // If panel key exists, keep plugin storage in sync for next standalone open.
  if (bootKey) persistManagementKey(bootKey);
  updateAuthState();
  const classLabel = {
    healthy: '健康', permission_denied: '权限被拒', quota_exhausted: '额度用尽',
    reauth: '需重新登录', model_unavailable: '模型不可用', probe_error: '探测异常', unknown: '未知'
  };
  const actionLabel = { keep: '保留', disable: '禁用', enable: '启用', delete: '删除' };
  const color = {
    healthy: '#047857', permission_denied: '#b45309', quota_exhausted: '#b45309',
    reauth: '#b91c1c', model_unavailable: '#475569', probe_error: '#b91c1c', unknown: '#475569'
  };
  function pill(text, c) {
    return '<span class="pill" style="background:' + c + '1a;color:' + c + '">' + escapeHtml(text) + '</span>';
  }
  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }
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
  function filtered() {
    const rows = state.snapshot.results || [];
    if (state.filter === 'all') return rows;
    // 「异常」= 探测异常 / 模型不可用 / 未知 等非主分类
    if (state.filter === 'other') {
      return rows.filter((r) => {
        const c = r.classification || '';
        return c !== 'healthy' && c !== 'permission_denied' && c !== 'quota_exhausted' && c !== 'reauth';
      });
    }
    return rows.filter((r) => r.classification === state.filter);
  }
  function filterLabel() {
    const map = {
      all: '全部',
      healthy: '健康',
      permission_denied: '权限被拒',
      quota_exhausted: '额度用尽',
      reauth: '需重登',
      other: '异常'
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
    if (o.reason) o.reason = decodeExportText(o.reason);
    return o;
  }
  function exportRows(format) {
    const rows = filtered().map(sanitizeExportRow);
    if (!rows.length) {
      showErr('当前筛选下没有可导出的数据');
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
      ['total','全部', summary.total || 0],
      ['healthy','健康', summary.healthy || 0],
      ['permission_denied','权限被拒', summary.permission_denied || 0],
      ['quota_exhausted','额度用尽', summary.quota_exhausted || 0],
      ['reauth','需重登', summary.reauth || 0],
      ['other','异常', summary.other || 0],
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
    $('exportHint').textContent = '当前分类：' + filterLabel() + '（' + rows.length + ' 条）';
    const totalPages = Math.max(1, Math.ceil(rows.length / state.pageSize));
    if (state.page > totalPages) state.page = totalPages;
    const start = (state.page - 1) * state.pageSize;
    const pageRows = rows.slice(start, start + state.pageSize);
    const tbody = $('rows');
    if (!pageRows.length) {
      tbody.innerHTML = '';
      $('empty').style.display = 'block';
      $('empty').textContent = hasManagementKey()
        ? '点击“开始巡检”检测 Grok 账号'
        : '请输入 CPA Management Key 后加载巡检状态';
    } else {
      $('empty').style.display = 'none';
      tbody.innerHTML = pageRows.map((r) => {
        const key = rowKey(r);
        const busy = pendingOps.has(key) || !!snap.applying;
        const toggleAct = r.disabled ? 'enable' : 'disable';
        const toggleLabel = r.disabled ? '启用' : '禁用';
        // Every row always offers toggle + delete (not only classification-suggested action).
        const actionBtns = hasManagementKey()
          ? '<div class="row-actions">' +
              '<button type="button" data-act="' + toggleAct + '" ' + (busy ? 'disabled' : '') + '>' + toggleLabel + '</button>' +
              '<button type="button" class="danger" data-act="delete" ' + (busy ? 'disabled' : '') + '>删除</button>' +
            '</div>'
          : '-';
        return '<tr data-key="' + escapeHtml(key) + '"' + (busy ? ' class="row-busy"' : '') + '>' +
          '<td class="col-name">' + escapeHtml(r.name) + '</td>' +
          '<td class="col-status">' + pill(r.disabled ? '已禁用' : '已启用', r.disabled ? '#b45309' : '#047857') + '</td>' +
          '<td class="col-result">' + pill(classLabel[r.classification] || r.classification || '-', color[r.classification] || '#475569') + '</td>' +
          '<td class="col-http">' + (r.http_status || '-') + '</td>' +
          '<td class="col-model">' + escapeHtml(r.model || '-') + '</td>' +
          '<td class="col-action">' + (actionLabel[r.action] || r.action || '-') + '</td>' +
          '<td class="col-reason">' + escapeHtml(r.reason || r.error_message || '-') + '</td>' +
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
      '<div style="font-size:12px;color:#64748b">显示 ' + from + '-' + to + ' / ' + rows.length +
      ' · 每页 <select id="pageSize">' +
      [20,50,100].map((n) => '<option value="' + n + '"' + (state.pageSize===n?' selected':'') + '>' + n + '</option>').join('') +
      '</select></div>' +
      '<div style="display:flex;gap:8px;align-items:center">' +
      '<button id="prev"' + (state.page<=1?' disabled':'') + '>上一页</button>' +
      '<span style="font-size:12px;color:#475569">' + state.page + ' / ' + totalPages + '</span>' +
      '<button id="next"' + (state.page>=totalPages?' disabled':'') + '>下一页</button></div>';
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
      $('filterRunBtn').textContent = filterCount ? ('巡检当前分类 (' + filterCount + ')') : '巡检当前分类';
    }
    $('stopBtn').disabled = !hasManagementKey() || !snap.running;
    $('applyBtn').disabled = !hasManagementKey() || busy || actionCount === 0;
    $('batchExportBtn').disabled = filteredCount === 0;
    $('batchDisableBtn').disabled = !hasManagementKey() || busy || disableCount === 0;
    $('batchEnableBtn').disabled = !hasManagementKey() || busy || enableCount === 0;
    $('batchDeleteBtn').disabled = !hasManagementKey() || busy || filteredCount === 0;
    $('applyBtn').textContent = snap.applying
      ? ('执行中 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0))
      : (actionCount ? ('执行建议操作 (' + actionCount + ')') : '执行建议操作');
    $('batchExportBtn').textContent = filteredCount ? ('批量导出 (' + filteredCount + ')') : '批量导出';
    $('batchDisableBtn').textContent = disableCount ? ('批量禁用 (' + disableCount + ')') : '批量禁用';
    $('batchEnableBtn').textContent = enableCount ? ('批量启用 (' + enableCount + ')') : '批量启用';
    $('batchDeleteBtn').textContent = filteredCount ? ('批量删除 (' + filteredCount + ')') : '批量删除';
    renderSchedule(snap.schedule);
    if (!hasManagementKey()) {
      setProgress('请输入 CPA Management Key 后加载巡检状态', false);
    } else if (snap.applying) {
      let msg = '后台执行操作 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0) + (snap.apply_current ? '：' + snap.apply_current : '');
      if ((snap.apply_failures || []).length) msg += '；失败 ' + snap.apply_failures.length;
      setProgress(msg, true);
    } else if (snap.running) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const mode = scoped ? '分类巡检中' : (snap.incremental ? '增量巡检中' : '巡检中');
      const extra = scoped ? '（仅当前分类，保留其他结果）' : (snap.incremental ? '（仅新增，保留已有结果）' : '（后台继续）');
      let phase = '';
      if (snap.probe_phase === 'retry') {
        phase = ' · 超时复检 ' + (snap.retry_done||0) + '/' + (snap.retry_total||0) + ' · 复检并发 ' + (snap.retry_workers||1);
      }
      setProgress(mode + ' ' + (snap.done||0) + '/' + (snap.total||0) + ' · 并发 ' + (snap.workers||WORKERS_DEFAULT) + phase + extra, true);
    } else if (snap.stopped) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const mode = scoped ? '分类已停止' : (snap.incremental ? '增量已停止' : '已停止');
      setProgress(mode + '，本轮 ' + (snap.done||0) + (snap.total ? '/' + snap.total : '') + '，列表共 ' + ((snap.results||[]).length) + ' 个账号', false);
    } else if ((snap.results||[]).length) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      let msg = '巡检完成，共 ' + (snap.results||[]).length + ' 个账号';
      if (scoped && (snap.done||0) >= 0 && snap.total != null) {
        msg = '分类完成：本轮检测 ' + (snap.done||0) + ' 个，列表共 ' + (snap.results||[]).length + ' 个';
      } else if (snap.incremental && (snap.done||0) >= 0 && snap.total != null) {
        msg = '增量完成：本轮新增检测 ' + (snap.done||0) + ' 个，列表共 ' + (snap.results||[]).length + ' 个';
      }
      if (snap.store_path) msg += ' · 已落盘';
      if ((snap.apply_failures || []).length) msg += ' · 上次操作失败 ' + snap.apply_failures.length + ' 条';
      setProgress(msg, false);
    } else {
      setProgress('等待开始', false);
    }
    if ((snap.apply_failures || []).length && !snap.applying) {
      // Always surface last op failures under the progress line (not below the table).
      showErr((snap.apply_failures || []).join('\n'));
    }
  }
  let pollTimer = null;
  const POLL_MS = 1200;
  const LIVE_RESULTS_MS = 2400;
  let lastJobBusy = false;
  let lastResultsGen = 0;
  let lastFullResultsAt = 0;
  let fullResultsSyncing = false;
  function stopPolling() {
    if (pollTimer != null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }
  function formatScheduleTime(iso) {
    if (!iso) return '—';
    try {
      const d = new Date(iso);
      if (Number.isNaN(d.getTime())) return iso;
      return d.toLocaleString();
    } catch (_) { return iso; }
  }
  function renderSchedule(sched) {
    const s = sched || (state.snapshot && state.snapshot.schedule) || {};
    if ($('schedEnabled')) $('schedEnabled').checked = !!s.enabled;
    if ($('schedInterval') && document.activeElement !== $('schedInterval')) {
      const n = Number(s.interval_minutes || 10);
      $('schedInterval').value = String(n > 0 ? n : 10);
    }
    if ($('schedAutoApply') && document.activeElement !== $('schedAutoApply')) {
      // default true when undefined (first load)
      $('schedAutoApply').checked = (s.auto_apply !== false);
    }
    const parts = [];
    if (s.enabled) {
      parts.push('定时已开启 · 每 ' + (s.interval_minutes || 10) + ' 分钟');
      parts.push(s.auto_apply ? '自动处置开' : '仅巡检不处置');
      parts.push('上次: ' + formatScheduleTime(s.last_run_at));
      parts.push('下次: ' + formatScheduleTime(s.next_run_at));
      if (s.last_auto_summary) parts.push('最近处置: ' + s.last_auto_summary);
      if (s.last_error) parts.push('错误: ' + s.last_error);
      if (!s.has_password) parts.push('⚠ 未检测到 Management Key（容器请设 MANAGEMENT_PASSWORD，或在此页保存过 Key）');
    } else {
      parts.push('定时未开启');
      if (s.last_run_at) parts.push('上次: ' + formatScheduleTime(s.last_run_at));
      if (s.last_auto_summary) parts.push('最近处置: ' + s.last_auto_summary);
      if (s.last_error) parts.push('错误: ' + s.last_error);
    }
    if ($('schedStatus')) $('schedStatus').textContent = parts.join(' · ');
    if ($('schedSaveBtn')) $('schedSaveBtn').disabled = !hasManagementKey();
  }
  async function saveSchedule() {
    if (!hasManagementKey()) {
      showErr('请先填写 CPA Management Key');
      return;
    }
    try {
      const interval = Number($('schedInterval').value || 10);
      if (!Number.isInteger(interval) || interval < 1 || interval > 1440) {
        throw new Error('间隔必须是 1-1440 的整数分钟');
      }
      const workers = normalizeWorkersInput(false);
      const body = {
        enabled: !!$('schedEnabled').checked,
        interval_minutes: interval,
        auto_apply: !!$('schedAutoApply').checked,
        workers: workers,
        include_disabled: true,
        only_disabled: false
      };
      const data = await api('/schedule', { method: 'PUT', body: JSON.stringify(body) });
      if (state.snapshot) state.snapshot.schedule = data;
      renderSchedule(data);
      showOk(body.enabled
        ? ('定时已保存：每 ' + interval + ' 分钟' + (body.auto_apply ? '，巡检后自动处置' : ''))
        : '定时已关闭');
      savePrefs({
        schedEnabled: body.enabled,
        schedInterval: interval,
        schedAutoApply: body.auto_apply
      });
    } catch (e) {
      showErr(String(e.message || e));
    }
  }
  function startPolling() {
    if (pollTimer != null) return;
    // Light polls omit results[] — progress only.
    pollTimer = setInterval(() => { refresh({ light: true }); }, POLL_MS);
  }
  // Only poll while a server job is active; idle pages do not keep hitting /status.
  function syncPolling(snap) {
    if (snap && (snap.running || snap.applying)) startPolling();
    else stopPolling();
  }
  function mergeLightStatus(data) {
    const prev = state.snapshot || {};
    // Keep local results during light poll; refresh meta/progress only.
    state.snapshot = Object.assign({}, prev, data || {}, {
      results: prev.results || [],
      include_results: false
    });
  }
  async function syncFullResults(force) {
    if (fullResultsSyncing) return false;
    if (!force && Date.now() - lastFullResultsAt < LIVE_RESULTS_MS) return false;
    fullResultsSyncing = true;
    try {
      await refresh({ light: false });
      return true;
    } finally {
      lastFullResultsAt = Date.now();
      fullResultsSyncing = false;
    }
  }
  async function refresh(opts) {
    const light = !!(opts && opts.light);
    if (!keyInput.value.trim()) {
      stopPolling();
      state.snapshot = { results: [], summary: {}, running: false, applying: false, done: 0, total: 0 };
      if ($('error').className === 'err') $('error').textContent = '';
      updateAuthState();
      render();
      return;
    }
    try {
      const path = light ? '/status?include_results=0' : '/status?include_results=1';
      const data = await api(path, { method: 'GET' });
      const busy = !!(data && (data.running || data.applying));
      const wasBusy = lastJobBusy;
      lastJobBusy = busy;

      if (light) {
        mergeLightStatus(data);
      } else {
        state.snapshot = data || {};
        if (data && data.results_gen != null) lastResultsGen = Number(data.results_gen) || 0;
        lastFullResultsAt = Date.now();
      }

      if (data && data.running) {
        $('includeDisabled').checked = !!data.include_disabled;
        $('onlyDisabled').checked = !!data.only_disabled;
        if (data.workers) $('workers').value = String(clampWorkers(Number(data.workers) || WORKERS_DEFAULT));
      }
      // Do not wipe success toasts; only clear stale red errors when server is clean.
      if (!(data.apply_failures || []).length && pendingOps.size === 0 && $('error').className === 'err') {
        $('error').textContent = '';
      }
      syncPolling(data);
      render();

      // Job just finished → pull full results once (list may have changed a lot).
      if (wasBusy && !busy) {
        await syncFullResults(true);
        return;
      }
      // Keep the account table live during inspection without sending the full
      // 1000+ row payload on every progress poll.
      if (light && data && data.results_gen != null) {
        const gen = Number(data.results_gen) || 0;
        if (gen && gen !== lastResultsGen) {
          const synced = await syncFullResults(!busy);
          if (synced) return;
        }
      }
    } catch (e) {
      showErr(String(e.message || e));
      // Keep polling only if we still believe a job is active.
      syncPolling(state.snapshot);
    }
  }
  function wireExclusive() {
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
  $('schedSaveBtn') && ($('schedSaveBtn').onclick = () => saveSchedule());
  $('runBtn').onclick = () => startInspection(false);
  $('incrBtn').onclick = () => startInspection(true);
  if ($('filterRunBtn')) $('filterRunBtn').onclick = () => startInspection('filter');
  $('stopBtn').onclick = async () => {
    try { await api('/stop', { method: 'POST', body: '{}' }); await refresh(); }
    catch (e) { showErr(String(e.message || e)); }
  };
  $('applyBtn').onclick = async () => {
    const actionCount = (state.snapshot.results || []).filter((r) => r.action === 'disable' || r.action === 'enable' || r.action === 'delete').length;
    const ok = await confirmDialog(
      '执行建议操作确认',
      '将对全部结果中「有建议动作」的账号异步执行禁用/启用/删除（共 ' + actionCount + ' 条建议）。\n' +
      '说明：此操作按建议执行，不受上方卡片当前分类限制。\n\n' +
      '请确认是否继续？'
    );
    if (!ok) return;
    try {
      const result = await api('/apply', { method: 'POST', body: '{}' });
      const total = Number(result && result.apply_total || 0);
      if (result && result.ok === false) throw new Error(result.error || '启动失败');
      showOk(total ? ('建议操作已在后台执行：共 ' + total + ' 项') : '建议操作已启动');
      await refresh();
    }
    catch (e) { showErr(String(e.message || e)); }
  };
  $('batchDisableBtn').onclick = () => batchForce('disable');
  $('batchEnableBtn').onclick = () => batchForce('enable');
  $('batchDeleteBtn').onclick = () => batchForce('delete');
  $('batchExportBtn').onclick = () => batchExport();
  $('confirmOk').onclick = () => closeConfirm(true);
  $('confirmCancel').onclick = () => closeConfirm(false);
  $('confirmModal').addEventListener('click', (ev) => {
    if (ev.target === $('confirmModal')) closeConfirm(false);
  });
  wireExclusive();
  // One-shot load on open; polling starts only when status reports running/applying.
  refresh();
  </script>
</body>
</html>`, base)
	return []byte(html)
}
