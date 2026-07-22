package main

import "fmt"

func renderUIPage(pluginID string) []byte {
	base := "/v0/management/plugins/" + pluginID
	html := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">Grok 账号巡检</title>
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
    label.ctl input[type=number],
    input[type=number] {
      width:56px; height:26px; border:1px solid #cbd5e1; border-radius:6px; padding:0 6px;
      background:#fff; color:#0f172a; -webkit-appearance:none; appearance:textfield;
    }
    label.ctl input[type=number]::-webkit-outer-spin-button,
    label.ctl input[type=number]::-webkit-inner-spin-button { -webkit-appearance:none; margin:0; }
    button { padding:0 12px; border:1px solid #d1d5db; background:#fff; color:#334155; cursor:pointer; }
    button.primary { border-color:#2563eb; background:#2563eb; color:#fff; font-weight:700; }
    button.soft { border-color:#c7d2fe; background:#eef2ff; color:#3730a3; font-weight:650; }
    button.danger { border-color:#fecaca; background:#fef2f2; color:#b91c1c; font-weight:650; }
    button:disabled { opacity:.55; cursor:not-allowed; }
    .summary { display:grid; grid-template-columns:repeat(6,minmax(100px,1fr)); gap:10px; margin-bottom:12px; }
    .summary.ban-summary { grid-template-columns:repeat(4,minmax(0,1fr)); width:100%%; min-width:0; }
    .card { background:#fff; border:1px solid #e2e8f0; border-radius:10px; padding:12px; box-shadow:0 1px 2px rgba(15,23,42,.04); cursor:pointer; min-width:0; }
    .card.active { outline:2px solid #2563eb; }
    .card .k { color:#64748b; font-size:12px; line-height:1.3; overflow-wrap:anywhere; word-break:break-word; }
    .card .v { margin-top:4px; font-size:22px; font-weight:750; }
    .bar { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:10px; align-items:center; }
    .actions-row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    .actions-row .hint { font-size:12px; color:var(--muted,#64748b); }
    .progress { min-height:20px; font-size:12px; color:#64748b; display:inline-flex; align-items:center; gap:6px; padding:4px 10px; border-radius:8px; max-width:100%%; }
    .progress.live { color:#1d4ed8; font-weight:700; background:#dbeafe; border:1px solid #93c5fd; box-shadow:0 0 0 1px rgba(37,99,235,.08); }
    .progress.live::before { content:""; width:8px; height:8px; border-radius:50%%; background:#2563eb; box-shadow:0 0 0 0 rgba(37,99,235,.55); animation:pulseDot 1.2s ease-out infinite; flex:0 0 auto; }
    @keyframes pulseDot { 0%% { box-shadow:0 0 0 0 rgba(37,99,235,.45); } 70%% { box-shadow:0 0 0 8px rgba(37,99,235,0); } 100%% { box-shadow:0 0 0 0 rgba(37,99,235,0); } }
    tr.row-out { opacity:0; transform:translateX(8px); transition:opacity .28s ease, transform .28s ease; }
    tr.row-busy { opacity:.55; }
    .row-actions { display:flex; gap:6px; flex-wrap:wrap; align-items:center; }
    .row-actions button { height:28px; padding:0 8px; font-size:12px; }
    .toast-ok { color:#047857; font-size:12px; margin-top:6px; }
    .modal { position:fixed; inset:0; z-index:10050; display:flex; align-items:center; justify-content:center; background:rgba(15,23,42,.45); padding:16px; }
    .modal.hidden { display:none; }
    .modal-card { width:min(440px,100%%); background:#fff; border-radius:12px; border:1px solid #e2e8f0; box-shadow:0 20px 40px rgba(15,23,42,.18); padding:18px 18px 14px; }
    .modal-title { font-size:16px; font-weight:700; color:#0f172a; margin-bottom:10px; }
    .modal-msg { font-size:13px; line-height:1.6; color:#334155; white-space:pre-wrap; margin-bottom:16px; }
    .modal-actions { display:flex; justify-content:flex-end; gap:8px; }
    .modal-actions button { min-width:76px; touch-action:manipulation; -webkit-tap-highlight-color:transparent; }
    .table-wrap { background:#fff; border:1px solid #e2e8f0; border-radius:10px; overflow:hidden; box-shadow:0 1px 2px rgba(15,23,42,.04); }
    .table-wrap .table-scroll { overflow:auto; -webkit-overflow-scrolling:touch; }
    table { width:100%%; border-collapse:collapse; min-width:1100px; font-size:13px; table-layout:auto; }
    .table-wrap.account-pool { width:100%%; min-width:0; }
    .table-wrap.account-pool .table-scroll { width:100%%; min-height:0; }
    .table-wrap.account-pool .empty {
      min-height:140px; display:flex; align-items:center; justify-content:center; box-sizing:border-box;
    }
    /* 巡检 / 自动禁用账号池同一套尺寸 */
    .table-wrap.account-pool table,
    .table-wrap.account-pool table.inspect-table,
    .table-wrap.account-pool table.ban-table {
      width:100%%; min-width:1100px; table-layout:auto; font-size:13px; border-collapse:collapse;
    }
    .table-wrap.account-pool table.ban-table th,
    .table-wrap.account-pool table.ban-table td,
    .table-wrap.account-pool table.inspect-table th,
    .table-wrap.account-pool table.inspect-table td {
      padding:10px 12px;
    }
    .table-wrap.account-pool table.ban-table td:first-child,
    .table-wrap.account-pool table.inspect-table td.col-name {
      word-break:break-all; overflow-wrap:anywhere;
    }
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
    .grok-inspection-page .hint,
    .grok-inspection-page .pager-meta {
      font-size: 12px;
      color: var(--muted, #64748b);
    }
    .grok-inspection-page .pager-meta-row {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }
    .grok-inspection-page select,
    .grok-inspection-page .pager select {
      height: 28px;
      border-radius: 6px;
      border: 1px solid var(--border, #e2e8f0);
      background: var(--surface, #fff);
      color: var(--text, #0f172a);
      color-scheme: inherit;
      padding: 0 6px;
      font-size: 12px;
    }
    html[data-grok-theme="dark"] .grok-inspection-page select,
    html[data-grok-theme="dark"] .grok-inspection-page .pager select {
      background: var(--surface, #1e293b) !important;
      color: var(--text, #e5e7eb) !important;
      border-color: var(--border, #334155) !important;
      color-scheme: dark;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .hint,
    html[data-grok-theme="dark"] .grok-inspection-page .pager-meta {
      color: var(--muted, #94a3b8) !important;
    }

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
    html:not([data-grok-theme="dark"]) .grok-inspection-page button.soft { background:#eef2ff !important; border-color:#c7d2fe !important; color:#3730a3 !important; }
    html:not([data-grok-theme="dark"]) .grok-inspection-page button.danger { background:#fef2f2 !important; border-color:#fecaca !important; color:#b91c1c !important; }
    .grok-inspection-page .modal-msg { color:var(--text) !important; }
    .grok-inspection-page input[type=number],
    .grok-inspection-page label.ctl input[type=number],
    .grok-inspection-page .key-row input {
      color:var(--text) !important;
      background:var(--surface-subtle, var(--surface)) !important;
      border-color:var(--input-border) !important;
      color-scheme: inherit;
    }
    html[data-grok-theme="dark"] .grok-inspection-page label.ctl {
      background:var(--surface) !important;
      border-color:var(--border) !important;
      color:var(--text) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page label.ctl input[type=number] {
      background:var(--surface-subtle) !important;
      color:var(--text) !important;
      border-color:var(--input-border) !important;
    }
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
    html[data-grok-theme="dark"] body,
    html[data-grok-theme="dark"] .grok-inspection-page {
      background: var(--page-bg) !important;
      color: var(--text) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .table-wrap,
    html[data-grok-theme="dark"] .grok-inspection-page .table-wrap.account-pool,
    html[data-grok-theme="dark"] .grok-inspection-page .card,
    html[data-grok-theme="dark"] .grok-inspection-page .module-bar,
    html[data-grok-theme="dark"] .grok-inspection-page .tabs,
    html[data-grok-theme="dark"] .grok-inspection-page .setting-card,
    html[data-grok-theme="dark"] .grok-inspection-page .modal-card {
      background: var(--surface) !important;
      border-color: var(--border) !important;
      color: var(--text) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page table,
    html[data-grok-theme="dark"] .grok-inspection-page thead,
    html[data-grok-theme="dark"] .grok-inspection-page tbody,
    html[data-grok-theme="dark"] .grok-inspection-page tr,
    html[data-grok-theme="dark"] .grok-inspection-page th,
    html[data-grok-theme="dark"] .grok-inspection-page td {
      background: transparent !important;
      color: var(--text) !important;
      border-color: var(--border-subtle) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page th {
      background: var(--surface-subtle) !important;
      color: var(--muted) !important;
      border-color: var(--border) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .pager {
      background: var(--surface-muted) !important;
      border-color: var(--border) !important;
      color: var(--muted) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .empty {
      background: transparent !important;
      color: var(--muted) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .key-row input,
    html[data-grok-theme="dark"] .grok-inspection-page input[type=number],
    html[data-grok-theme="dark"] .grok-inspection-page label.ctl {
      background: var(--surface-subtle) !important;
      color: var(--text) !important;
      border-color: var(--input-border) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page button:not(.primary):not(.soft):not(.danger):not(.tab) {
      background: var(--surface) !important;
      border-color: var(--border) !important;
      color: var(--text) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page button.soft { background:#242c58 !important; border-color:#4b5aa6 !important; color:#dbe4ff !important; }
    html[data-grok-theme="dark"] .grok-inspection-page button.danger { background:#3f1d1d !important; border-color:#7f1d1d !important; color:#fecaca !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .badge { background:#252b63 !important; color:#c7d2fe !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .card.active { outline-color:#60a5fa !important; }
    html[data-grok-theme="dark"] .modal-card { background:#1e293b !important; border-color:#334155 !important; color:#e5e7eb !important; }
    html[data-grok-theme="dark"] .modal-title { color:#f8fafc !important; }
    html[data-grok-theme="dark"] .modal-msg { color:#cbd5e1 !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .progress.live { color:#93c5fd !important; background:#1e3a5f !important; border-color:#3b82f6 !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .progress.live::before { background:#60a5fa; }
    /* Fallback when host theme attribute is missing but OS/browser is dark */
    @media (prefers-color-scheme: dark) {
      html:not([data-grok-theme="light"]) {
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
    }
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
      .grok-inspection-page .controls > #filterRunBtn { grid-column:1 / -1; grid-row:7; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .summary,
      .grok-inspection-page .summary.ban-summary {
        grid-template-columns:repeat(2,minmax(0,1fr)) !important;
        gap:8px; width:100%%; min-width:0;
      }
      .grok-inspection-page .card {
        min-width:0; min-height:74px; padding:10px 12px; overflow:hidden;
        display:flex; flex-direction:column; justify-content:center; box-sizing:border-box;
      }
      .grok-inspection-page .card .k { font-size:11px; line-height:1.25; }
      .grok-inspection-page .card .v { font-size:22px; margin-top:4px; line-height:1.15; }
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
      .grok-inspection-page .tabs {
        width:100%% !important; max-width:100%% !important;
        display:grid !important; grid-template-columns:1fr 1fr !important; gap:6px;
        box-sizing:border-box;
      }
      .grok-inspection-page .tab,
      .grok-inspection-page button.tab {
        flex:none !important; width:100%% !important; min-width:0 !important;
        max-width:none !important; align-items:flex-start !important;
        box-sizing:border-box;
      }
      .grok-inspection-page .table-wrap.account-pool table,
      .grok-inspection-page .table-wrap.account-pool table.inspect-table,
      .grok-inspection-page .table-wrap.account-pool table.ban-table {
        min-width:720px !important;
      }
      .grok-inspection-page .table-wrap.account-pool { border-radius:10px; width:100%%; }
      .grok-inspection-page .table-wrap.account-pool .empty { min-height:120px; }
      .grok-inspection-page .table-wrap.account-pool code {
        font:inherit; background:transparent; border:0; padding:0; color:inherit;
      }
    }
    .grok-inspection-page .tabs {
      display:inline-flex; gap:6px; flex-wrap:wrap; margin:0 0 14px; padding:3px;
      background:var(--surface,#fff); border:1px solid var(--border,#e2e8f0); border-radius:10px;
      width:fit-content; max-width:100%%; box-sizing:border-box;
    }
    .grok-inspection-page .tab,
    .grok-inspection-page button.tab {
      border:1px solid transparent !important;
      background:transparent !important;
      color:var(--muted,#64748b) !important;
      padding:8px 12px !important;
      border-radius:8px !important;
      cursor:pointer;
      font:inherit;
      font-weight:600 !important;
      height:auto !important;
      min-height:0 !important;
      display:flex !important;
      flex-direction:column !important;
      align-items:flex-start !important;
      gap:1px;
      flex:0 0 auto !important;
      min-width:0 !important;
      width:auto !important;
      max-width:none;
      box-shadow:none !important;
      outline:none !important;
      -webkit-appearance:none;
      appearance:none;
    }
    .grok-inspection-page .tab-title { font-size:13px; line-height:1.2; color:inherit !important; white-space:nowrap; }
    .grok-inspection-page .tab-desc { font-size:11px; font-weight:500; opacity:.78; line-height:1.2; color:inherit !important; white-space:nowrap; }
    .grok-inspection-page .tab.active,
    .grok-inspection-page .tab.active:hover,
    .grok-inspection-page .tab.active:focus,
    .grok-inspection-page .tab.active:focus-visible,
    .grok-inspection-page .tab.active:active,
    .grok-inspection-page button.tab.active,
    .grok-inspection-page button.tab.active:hover,
    .grok-inspection-page button.tab.active:focus,
    .grok-inspection-page button.tab.active:focus-visible,
    .grok-inspection-page button.tab.active:active {
      background:#2563eb !important;
      color:#fff !important;
      border-color:#2563eb !important;
      box-shadow:0 1px 2px rgba(37,99,235,.25) !important;
    }
    .grok-inspection-page .tab.active .tab-title,
    .grok-inspection-page .tab.active .tab-desc { color:#fff !important; opacity:1; }
    .grok-inspection-page .tab:not(.active),
    .grok-inspection-page button.tab:not(.active) {
      background:transparent !important;
      color:var(--muted,#64748b) !important;
    }
    .grok-inspection-page .tab:not(.active):hover,
    .grok-inspection-page button.tab:not(.active):hover {
      background:var(--surface-subtle,#f1f5f9) !important;
      color:var(--text,#0f172a) !important;
      border-color:var(--border,#e2e8f0) !important;
    }
    .panel { display:none; }
    .panel.active { display:block; }
    .shared-key { margin-bottom:12px; }
    .module-bar { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; margin:0 0 12px; padding:12px 14px; border:1px solid var(--border,#e2e8f0); border-radius:12px; background:var(--surface,#fff); }
    .module-bar h2 { margin:0; font-size:15px; }
    .module-bar .module-sub { margin:2px 0 0; font-size:12px; color:var(--muted,#64748b); }
    .switch-row { display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
    .switch { position:relative; display:inline-block; width:46px; height:26px; flex:0 0 auto; }
    .switch input { opacity:0; width:0; height:0; }
    .slider { position:absolute; cursor:pointer; inset:0; background:#cbd5e1; transition:.18s; border-radius:999px; }
    .slider:before { position:absolute; content:""; height:20px; width:20px; left:3px; top:3px; background:#fff; transition:.18s; border-radius:50%%; box-shadow:0 1px 2px rgba(15,23,42,.2); }
    .switch input:checked + .slider { background:#16a34a; }
    .switch input:checked + .slider:before { transform:translateX(20px); }
    .switch input:disabled + .slider { opacity:.55; cursor:not-allowed; }
    .status-pill { display:inline-flex; align-items:center; padding:3px 10px; border-radius:999px; font-size:12px; font-weight:700; }
    .status-pill.on { background:#dcfce7; color:#166534; }
    .status-pill.off { background:#fee2e2; color:#991b1b; }
    .autoban-split { display:grid; grid-template-columns:1fr; gap:12px; margin-bottom:12px; }
    .setting-card { background:var(--surface,#fff); border:1px solid var(--border,#e2e8f0); border-radius:10px; padding:14px; box-shadow:0 1px 2px rgba(15,23,42,.04); }
    .setting-card h3 { margin:0 0 8px; font-size:15px; }
    .setting-card p { margin:0 0 12px; color:var(--muted,#64748b); font-size:13px; line-height:1.5; }
    .stat-line { display:flex; justify-content:space-between; gap:10px; padding:8px 0; border-bottom:1px solid #f1f5f9; font-size:13px; }
    .stat-line:last-child { border-bottom:0; }
    .settings-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }
    .setting-row { display:flex; gap:10px; align-items:center; flex-wrap:wrap; margin-bottom:10px; font-size:13px; }
    .setting-row label { min-width:140px; }
    .note { margin-top:12px; padding:12px 14px; border-radius:10px; background:#eff6ff; border:1px solid #bfdbfe; color:#1e3a8a; font-size:13px; line-height:1.55; }
    html[data-grok-theme="dark"] .grok-inspection-page .tab:not(.active):hover, html[data-grok-theme="dark"] .grok-inspection-page button.tab:not(.active):hover { background:#1a2332 !important; color:#e5e7eb !important; border-color:#334155 !important; }
    html[data-grok-theme="dark"] .setting-card, html[data-grok-theme="dark"] .tabs { background:var(--surface) !important; border-color:var(--border) !important; }
    html[data-grok-theme="dark"] .note { background:#1e293b; border-color:#334155; color:#bfdbfe; }
    html[data-grok-theme="dark"] .stat-line { border-color:#2b3648; }
    html[data-grok-theme="dark"] .module-bar { background:var(--surface) !important; border-color:var(--border) !important; }
    html[data-grok-theme="dark"] .status-pill.on { background:#14532d; color:#bbf7d0; }
    html[data-grok-theme="dark"] .status-pill.off { background:#7f1d1d; color:#fecaca; }
    html[data-grok-theme="dark"] .slider { background:#475569; }
    @media (min-width:761px) and (max-width:960px) {
      .grok-inspection-page .tabs { width:fit-content; max-width:100%%; }
      .grok-inspection-page .tab,
      .grok-inspection-page button.tab { flex:0 0 auto !important; width:auto !important; }
      .autoban-split, .settings-grid { grid-template-columns:1fr; }
    }
    /* 手机端最终覆盖：保证 tabs 铺满，且两个账号池同宽 */
    @media (max-width:760px) {
      .grok-inspection-page .tabs {
        width:100%% !important; max-width:100%% !important;
        display:grid !important; grid-template-columns:1fr 1fr !important; gap:6px !important;
        box-sizing:border-box !important;
      }
      .grok-inspection-page .tab,
      .grok-inspection-page button.tab {
        flex:none !important; width:100%% !important; min-width:0 !important;
        max-width:none !important; box-sizing:border-box !important;
      }
      .grok-inspection-page .table-wrap.account-pool {
        width:100%% !important; min-width:0 !important;
      }
      .grok-inspection-page .table-wrap.account-pool table,
      .grok-inspection-page .table-wrap.account-pool table.inspect-table,
      .grok-inspection-page .table-wrap.account-pool table.ban-table {
        min-width:720px !important;
      }
      .grok-inspection-page .table-wrap.account-pool table.ban-table td.col-name,
      .grok-inspection-page .table-wrap.account-pool table.inspect-table td.col-name {
        word-break:break-all; overflow-wrap:anywhere;
      }
      .grok-inspection-page .table-wrap.account-pool code {
        font:inherit !important; background:transparent !important; border:0 !important; padding:0 !important; color:inherit !important;
      }
    }
  </style>
</head>
<body>
  <div class="wrap grok-inspection-page">
    <div class="hero">
      <div>
        <div class="badge">xAI / Grok · CPA Plugin</div>
        <h1 data-i18n="title">Grok 账号巡检</h1>
        <p class="sub" id="heroSub" data-i18n="subtitle">「开始巡检」清空并重测全部；「增量巡检」只测新增账号；「巡检当前分类」只重测所选分类（需先点分类卡片）；「批量操作」只作用于当前筛选；结果会自动保存。</p>
      </div>
      <div class="controls">
        <label class="ctl"><span data-i18n="language">语言</span>
          <select id="langSelect" style="height:26px;border:1px solid #cbd5e1;border-radius:6px;padding:0 4px;">
            <option value="zh">中文</option>
            <option value="en">English</option>
          </select>
        </label>
      </div>
    </div>
    <div class="controls shared-key" id="keyRow">
      <div class="key-row" style="flex:1;min-width:min(360px,100%%)">
        <input id="managementKey" type="password" autocomplete="current-password" data-i18n-placeholder="key_label" placeholder="CPA Management Key（可自动读取管理面板）">
        <span class="hint" id="keyHint"></span>
      </div>
    </div>
    <div class="tabs" role="tablist" aria-label="功能页签" data-i18n-title="tabs_aria">
      <button class="tab active" type="button" data-tab="inspect" id="tabInspect" aria-selected="true" role="tab"><span class="tab-title" data-i18n="tab_inspect">账号巡检</span><span class="tab-desc" data-i18n="tab_inspect_desc">批量探测 · 建议操作</span></button>
      <button class="tab" type="button" data-tab="autoban" id="tabAutoban" aria-selected="false" role="tab"><span class="tab-title" data-i18n="tab_autoban">实时自动禁用</span><span class="tab-desc" data-i18n="tab_autoban_desc">请求拦截 · 定时恢复</span></button>

    </div>
    <section class="panel active" id="panel-inspect">
    <div class="controls">
      <label class="ctl"><span data-i18n="workers">并发</span> <input id="workers" type="number" min="1" max="16" step="1" value="6" title="1-16 的整数"></label>
      <label class="ctl"><input id="includeDisabled" type="checkbox"> <span data-i18n="include_disabled">包含已禁用</span></label>
      <label class="ctl"><input id="onlyDisabled" type="checkbox"> <span data-i18n="only_disabled">仅巡检已禁用</span></label>
      <button id="stopBtn" disabled data-i18n="stop">停止</button>
      <button id="applyBtn" class="soft" disabled data-i18n="apply_suggested">执行建议操作</button>
      <button id="incrBtn" class="soft" disabled data-i18n-title="incremental_title" title="只检测 Auth 中相对上次结果新增的账号" data-i18n="incremental">增量巡检</button>
      <button id="filterRunBtn" class="soft" disabled data-i18n-title="category_title" title="只重新探测当前卡片筛选分类下的账号，保留其他结果" data-i18n="inspect_category">巡检当前分类</button>
      <button id="runBtn" class="primary" data-i18n="start">开始巡检</button>
    </div>
    <div id="summary" class="summary"></div>
    <div class="bar">
      <div class="actions-row">
        <button id="batchExportBtn" type="button" disabled data-i18n="bulk_export">批量导出</button>
        <button id="batchDisableBtn" class="soft" type="button" disabled data-i18n="bulk_disable">批量禁用</button>
        <button id="batchEnableBtn" class="soft" type="button" disabled data-i18n="bulk_enable">批量启用</button>
        <button id="batchDeleteBtn" class="danger" type="button" disabled data-i18n="bulk_delete">批量删除</button>
        <span class="hint" id="exportHint" data-i18n="filter_hint">点击上方卡片切换分类；禁用/启用数量按当前分类下列表的启用/禁用状态统计</span>
      </div>
      <div style="display:flex;flex-direction:column;align-items:flex-end;gap:4px;min-width:0;max-width:100%%">
        <div id="progress" class="progress" data-i18n="waiting">等待开始</div>
        <pre id="error" class="err" style="margin:0;max-width:min(720px,100%%);text-align:left;font-size:12px;line-height:1.45;white-space:pre-wrap;word-break:break-word"></pre>
      </div>
    </div>

      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="inspect-table">
            <thead>
              <tr>
                <th class="col-name" data-i18n="th_account">账号</th><th class="col-status" data-i18n="th_status">当前状态</th><th class="col-result" data-i18n="th_result">检测结果</th><th class="col-http" data-i18n="th_http">HTTP</th><th class="col-model" data-i18n="th_model">模型</th><th class="col-action" data-i18n="th_action">建议</th><th class="col-reason" data-i18n="th_reason">原因</th><th class="col-ops" data-i18n="th_ops">操作</th>
              </tr>
            </thead>
            <tbody id="rows"></tbody>
          </table>
        </div>
        <div id="empty" class="empty" data-i18n="need_key_load">请输入 CPA Management Key 后加载巡检状态</div>
        <div id="pager" class="pager"></div>
      </div>
    </section>

    <section class="panel" id="panel-autoban">
      <div class="module-bar">
        <div>
          <h2 data-i18n="ban_title">实时自动禁用</h2>
        </div>
        <div class="switch-row">
          <label class="switch" data-i18n-title="ban_enable" title="开启后实时拦截并禁用">
            <input id="banEnabledToggle" type="checkbox">
            <span class="slider"></span>
          </label>
          <span id="banEnabledPill" class="status-pill off" data-i18n="ban_off">已关闭</span>
          <span class="hint" id="banEnabledHint" class="hint" data-i18n="ban_enabled_hint">开关会立即生效并保存</span>
        </div>
      </div>
      <div class="controls" style="margin-bottom:12px">
        <button id="banRefreshBtn" class="soft" type="button" data-i18n="ban_refresh">刷新状态</button>
        <button id="banUnbanFilterBtn" class="soft" type="button" disabled data-i18n="ban_unban_filter">解禁当前分类</button>
        <button id="banUnbanAllBtn" class="danger" type="button" data-i18n="ban_unban_all">全部解禁</button>
        <span class="hint" id="banFilterHint" class="hint" data-i18n="ban_filter_hint">点击下方卡片筛选分类</span>
      </div>
      <div id="banSummary" class="summary ban-summary">
        <div class="card active" data-ban-filter="all"><div class="k" data-i18n="ban_all">全部</div><div class="v" id="banCount">0</div></div>
        <div class="card" data-ban-filter="quota"><div class="k" data-i18n="ban_quota">额度用尽</div><div class="v" id="banQuotaCount">0</div></div>
        <div class="card" data-ban-filter="permission"><div class="k" data-i18n="ban_permission">权限拒绝</div><div class="v" id="banPermissionCount">0</div></div>
        <div class="card" data-ban-filter="unauthorized"><div class="k" data-i18n="ban_authfail">401 认证失败</div><div class="v" id="banUnauthorizedCount">0</div></div>
      </div>
      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="ban-table">
            <thead>
              <tr>
                <th class="col-name" data-i18n="th_account">账号</th><th data-i18n="ban_th_reason">禁用原因</th><th data-i18n="ban_th_time">禁用时间</th><th data-i18n="ban_th_restore">恢复方式</th><th data-i18n="ban_th_remain">剩余</th><th class="col-ops" data-i18n="th_ops">操作</th>
              </tr>
            </thead>
            <tbody id="banRows"></tbody>
          </table>
        </div>
        <div id="banEmpty" class="empty" data-i18n="ban_status_loading">加载中…</div>
        <div id="banPager" class="pager"></div>
      </div>
      <pre id="banError" class="err" style="margin-top:10px;font-size:12px;white-space:pre-wrap"></pre>
    </section>
    <div id="confirmModal" class="modal hidden" aria-hidden="true">
      <div class="modal-card" role="dialog" aria-modal="true">
        <div id="confirmTitle" class="modal-title" data-i18n="confirm_title">确认操作</div>
        <div id="confirmMsg" class="modal-msg"></div>
        <div class="modal-actions">
          <button type="button" id="confirmCancel" data-i18n="cancel">取消</button>
          <button type="button" id="confirmOk" class="primary" data-i18n="ok">确定</button>
        </div>
      </div>
    </div>

    </div>
  <script>
  const I18N = {
    zh: {
      tab_inspect:'账号巡检', tab_inspect_desc:'批量探测 · 建议操作',
      tab_autoban:'实时自动禁用', tab_autoban_desc:'请求拦截 · 定时恢复',
      tabs_aria:'功能页签',
      ban_title:'实时自动禁用',
      ban_enable:'开启后实时拦截并禁用',
      ban_on:'已开启', ban_off:'已关闭',
      ban_enabled_hint:'开关会立即生效并保存',
      ban_refresh:'刷新状态', ban_unban_filter:'解禁当前分类', ban_unban_all:'全部解禁',
      ban_filter_hint:'点击下方卡片筛选分类',
      ban_all:'全部', ban_quota:'额度用尽', ban_permission:'权限拒绝', ban_authfail:'401 认证失败',
      ban_manual:'需手动解禁', ban_auto_restore:'定时自动恢复',
      ban_th_account:'账号', ban_th_reason:'禁用原因', ban_th_time:'禁用时间', ban_th_restore:'恢复方式', ban_th_remain:'剩余', ban_th_until:'恢复时间', ban_th_ops:'操作',
      ban_empty:'当前没有自动禁用中的账号',
      ban_unban:'解禁',
      ban_status_loading:'加载中…',

      title:'Grok 账号巡检',
      subtitle:'「开始巡检」清空并重测全部；「增量巡检」只测新增账号；「巡检当前分类」只重测所选分类（需先点分类卡片）；「批量操作」只作用于当前筛选；结果会自动保存。',
      language:'语言', key_label:'CPA Management Key（可自动读取管理面板）', workers:'并发',
      include_disabled:'包含已禁用', only_disabled:'仅巡检已禁用', stop:'停止', apply_suggested:'执行建议操作',
      incremental:'增量巡检', inspect_category:'巡检当前分类', start:'开始巡检',
      incremental_title:'只检测 Auth 中相对上次结果新增的账号', category_title:'只重新探测当前卡片筛选分类下的账号，保留其他结果',
      bulk_export:'批量导出', bulk_disable:'批量禁用', bulk_enable:'批量启用', bulk_delete:'批量删除',
      filter_hint:'点击上方卡片切换分类；禁用/启用数量按当前分类下列表的启用/禁用状态统计',
      waiting:'等待开始', confirm_title:'确认操作', cancel:'取消', ok:'确定',
      th_account:'账号', th_status:'当前状态', th_result:'检测结果', th_http:'HTTP', th_model:'模型', th_action:'建议', th_reason:'原因', th_ops:'操作',
      class_healthy:'健康', class_permission_denied:'权限被拒', class_quota_exhausted:'额度用尽',
      class_reauth:'需重新登录', class_model_unavailable:'模型不可用', class_probe_error:'探测异常', class_unknown:'未知',
      class_all:'全部', class_other:'异常',
      action_keep:'保留', action_disable:'禁用', action_enable:'启用', action_delete:'删除',
      enabled:'已启用', disabled:'已禁用', failed:'失败', running:'执行中',
      need_key:'请先填写 CPA Management Key', need_key_load:'请输入 CPA Management Key 后加载巡检状态',
      click_start:'点击“开始巡检”检测 Grok 账号',
      workers_int_prefix:'并发必须是 ', workers_int_mid:' 的整数（当前默认 ', workers_int_suffix:'）',
      workers_range_prefix:'并发必须在 ', workers_range_suffix:' 之间',
      key_from_panel:'已从管理面板自动读取 Key（无需手填）', key_autofill:'已自动填充（可改）',
      key_local:'已使用本插件本地保存的 Key',
      key_missing:'未读到 Key：请先登录 /management.html 并勾选记住密码，或在此手动填写',
      pick_category_first:'请先点击分类卡片选择一个分类，再巡检当前分类',
      no_cat_prefix:'当前分类「', no_cat_suffix:'」下没有可巡检账号', cat_under:'」下', confirm_suffix:'确认',
      still_running:'仍在执行…', action_timeout:'操作超时，请刷新后确认是否已生效',
      delete_confirm_title:'删除确认',
      delete_body_prefix:'将删除 CPA Auth 凭证「', delete_body_suffix:'」。\n此操作不可恢复，确认继续？',
      no_enabled:'没有「已启用」可禁用的账号', no_disabled:'没有「已禁用」可启用的账号', no_actionable:'没有可操作的账号',
      only_enabled_rows:'仅包含当前列表中状态为「已启用」的账号。',
      only_disabled_rows:'仅包含当前列表中状态为「已禁用」的账号。',
      all_in_category:'包含当前分类下全部账号。',
      bulk_delete_api:'将调用 CPA 本体批量删除接口（DELETE /auth-files，每批最多 50 个），并更新本地结果。\n',
      irreversible:'此操作不可恢复。',
      bulk_patch_prefix:'将通过 CPA Management API ', bulk_patch_suffix:'账号，并更新本地结果。\n',
      bulk_patch_note:'说明：CPA 本体没有批量启用/禁用接口，只能逐个调用 PATCH（插件侧会并发约 6 路），',
      bulk_patch_slow:'账号多时可能较慢，上方会显示进度。\n',
      bulk_patch_hint:'若需要更快清理，可改用「批量删除」（本体支持一次删多个）。',
      bulk_word:'批量', current_category:'当前分类：', affected:'影响账号：', units:' 个\n',
      will_bulk:'将对上述账号执行批量', continue_q:'请确认是否继续？',
      started_prefix:'已启动：共 ', started_suffix:' 项（后台执行，进度见上方状态）',
      no_export_suffix:'」下没有可导出的数据', export_confirm:'批量导出确认',
      export_count:'导出条数：', export_rows:' 条\n\n',
      export_body:'将导出当前分类下的全部账号（不是仅当前页）为 JSON 文件。\n\n',
      no_export_filter:'当前筛选下没有可导出的数据',
      showing:'显示 ', per_page:' · 每页 ', prev:'上一页', next:'下一页',
      inspect_running:'巡检中', category_running:'分类巡检中', incremental_running:'增量巡检中',
      category_only_keep:'（仅当前分类，保留其他结果）', incremental_only_keep:'（仅新增，保留已有结果）', bg_continue:'（后台继续）',
      timeout_recheck:' · 超时复检 ', recheck_workers:' · 复检并发 ', workers_sep:' · 并发 ',
      category_stopped:'分类已停止', incremental_stopped:'增量已停止', stopped:'已停止',
      this_run:'，本轮 ', list_total:'，列表共 ', accounts_word:' 个账号',
      inspection_complete:'巡检完成，共 ', category_complete:'分类完成：本轮检测 ', list_mid:' 个，列表共 ', list_end:' 个',
      incremental_complete:'增量完成：本轮新增检测 ', persisted:' · 已落盘', last_fail:' · 上次操作失败 ', rows_word:' 条', rows_paren:' 条）',
      apply_confirm_title:'执行建议操作确认',
      apply_body_prefix:'将对全部结果中「有建议动作」的账号异步执行禁用/启用/删除（共 ',
      apply_body_suffix:' 条建议）。\n说明：此操作按建议执行，不受上方卡片当前分类限制。\n\n',
      start_failed:'启动失败', suggested_running_prefix:'建议操作已在后台执行：共 ', items_word:' 项',
      suggested_started:'建议操作已启动', bg_actions:'后台执行操作 ', failed_sep:'；失败 ',
      no_action_seq:'服务端未返回 action_seq，无法确认执行结果',
      deleted_prefix:'删除成功：', disabled_prefix:'禁用成功：', enabled_prefix:'启用成功：', running_prefix:'执行中：',
      key_manual:'已使用手动填写的 Key',
      hero_autoban:'右侧开关控制实时自动禁用。命中额度用尽 / 权限拒绝 / 401 时自动禁用账号；额度用尽默认 24 小时后恢复，其余需手动解禁。',
      filter_count_prefix:'当前分类：', filter_count_mid:'（', filter_count_suffix:' 条）',
      action_failed_suffix:'失败',
      bulk_confirm_title_prefix:'批量', bulk_confirm_title_suffix:'确认',
      bulk_started_prefix:'批量',
      no_cat_action_mid:'」下',
      export_hint_prefix:'当前分类：',
      unban_running:'解禁进行中 ', unban_fail_sep:' · 失败 ', unban_persist_sep:' · 保存失败: ',
      apply_progress:'后台执行操作 ',
      persist_fail_sep:' · 保存失败: ',
      unban_progress_complete_fail:'保存自动禁用状态失败: ',
      hours_minutes_mid:' 小时 ', hours_minutes_suffix:' 分', minutes_suffix:' 分',
      ban_unknown_reason:'未知原因',
      ban_reason_quota:'额度用尽', ban_reason_permission:'权限被拒绝', ban_reason_authfail:'认证失败',
      restore_header_absolute:'按上游重置时间自动恢复',
      restore_header_relative:'按 Retry-After 自动恢复',
      restore_date_plus_fallback:'冷却窗口自动恢复',
      restore_local_plus_fallback:'定时自动恢复',
      ban_unban_filter_named_prefix:'解禁「', ban_unban_filter_named_suffix:'」',
      ban_filter_hint_all:'点击下方卡片筛选分类；解禁当前分类作用于全部禁用账号',
      ban_filter_current_prefix:'当前筛选：', ban_filter_current_mid:' · 共 ', ban_filter_current_suffix:' 个',
      ban_empty_filter_prefix:'当前分类「', ban_empty_filter_suffix:'」没有账号',
      ban_need_key_load:'请输入 CPA Management Key 后加载自动禁用状态',
      ban_not_loaded:'未加载', ban_running:'运行中',
      pager_total_prefix:'共 ', pager_total_suffix:' 个', pager_page_mid:' · 第 ', pager_page_of:' / ', pager_page_suffix:' 页',
      unban_confirm_title:'确认解禁', unban_confirm_body_prefix:'将重新启用账号：\n',
      unban_failed:'解禁失败', unban_success_title:'解禁成功',
      unban_missing:'账号已不在 CPA 认证列表中，已清除本插件禁用记录',
      unban_success_msg:'已解禁并重新启用',
      unban_filter_empty:'当前分类没有可解禁账号',
      unban_filter_confirm_title:'确认解禁当前分类',
      unban_filter_confirm_body_prefix:'将解禁「', unban_filter_confirm_body_mid:'」下的 ', unban_filter_confirm_body_suffix:' 个账号。\n后台异步执行，可用停止按钮中止。',
      unban_in_progress:'解禁中…', unban_start_failed:'启动解禁失败',
      unban_filter_started_prefix:'分类解禁已在后台执行：共 ', unban_filter_started_suffix:' 个',
      unban_all_confirm_title:'确认全部解禁',
      unban_all_confirm_body:'将尝试解禁当前禁用池中的全部账号。\n后台异步执行，可用停止按钮中止。',
      unban_all_started:'全部解禁已在后台执行',
      notice_title:'提示',
      apply_disable_line:'· 禁用 ', apply_enable_line:'· 启用 ', apply_delete_line:'· 删除 ',
      apply_body_counts_mid:' 个\n',
    },
    en: {
      tab_inspect:'Account inspection', tab_inspect_desc:'Batch probe · suggested actions',
      tab_autoban:'Realtime auto-ban', tab_autoban_desc:'Request intercept · scheduled restore',
      tabs_aria:'Feature tabs',
      ban_title:'Realtime auto-ban',
      ban_enable:'When on, intercept and ban in realtime',
      ban_on:'On', ban_off:'Off',
      ban_enabled_hint:'Toggle applies immediately and is saved',
      ban_refresh:'Refresh status', ban_unban_filter:'Unban current filter', ban_unban_all:'Unban all',
      ban_filter_hint:'Click a card below to filter',
      ban_all:'All', ban_quota:'Quota exhausted', ban_permission:'Permission denied', ban_authfail:'401 auth failed',
      ban_manual:'Manual unban required', ban_auto_restore:'Auto restore on schedule',
      ban_th_account:'Account', ban_th_reason:'Ban reason', ban_th_time:'Banned at', ban_th_restore:'Restore mode', ban_th_remain:'Remaining', ban_th_until:'Restore at', ban_th_ops:'Actions',
      ban_empty:'No accounts are currently auto-banned',
      ban_unban:'Unban',
      ban_status_loading:'Loading…',

      title:'Grok Account Inspection',
      subtitle:'"Start inspection" clears and rechecks all accounts; "Incremental inspection" only checks newly added accounts; "Inspect current category" only rechecks the selected category (click a category card first); bulk actions apply only to the current filter; results are saved automatically.',
      language:'Language', key_label:'CPA Management Key (can auto-read from the management panel)', workers:'Workers',
      include_disabled:'Include disabled', only_disabled:'Only inspect disabled', stop:'Stop', apply_suggested:'Apply suggested actions',
      incremental:'Incremental inspection', inspect_category:'Inspect current category', start:'Start inspection',
      incremental_title:'Only probe accounts newly present in Auth since the last result set', category_title:'Only re-probe accounts in the currently selected category card; keep other results',
      bulk_export:'Bulk export', bulk_disable:'Bulk disable', bulk_enable:'Bulk enable', bulk_delete:'Bulk delete',
      filter_hint:'Click a category card above to filter; disable/enable counts use the enabled/disabled state of the current category list',
      waiting:'Waiting to start', confirm_title:'Confirm action', cancel:'Cancel', ok:'OK',
      th_account:'Account', th_status:'Current status', th_result:'Probe result', th_http:'HTTP', th_model:'Model', th_action:'Suggestion', th_reason:'Reason', th_ops:'Actions',
      class_healthy:'Healthy', class_permission_denied:'Permission denied', class_quota_exhausted:'Quota exhausted',
      class_reauth:'Reauth required', class_model_unavailable:'Model unavailable', class_probe_error:'Probe error', class_unknown:'Unknown',
      class_all:'All', class_other:'Other',
      action_keep:'Keep', action_disable:'Disable', action_enable:'Enable', action_delete:'Delete',
      enabled:'Enabled', disabled:'Disabled', failed:'Failed', running:'Running',
      need_key:'Enter the CPA Management Key first', need_key_load:'Enter the CPA Management Key to load inspection status',
      click_start:'Click "Start inspection" to probe Grok accounts',
      workers_int_prefix:'Workers must be an integer ', workers_int_mid:' (current default ', workers_int_suffix:')',
      workers_range_prefix:'Workers must be between ', workers_range_suffix:'',
      key_from_panel:'Key auto-loaded from the management panel (no manual entry needed)', key_autofill:'Auto-filled (editable)',
      key_local:'Using a key previously saved by this plugin',
      key_missing:'No key found: log in at /management.html with remember-password, or enter it here',
      pick_category_first:'Click a category card first, then inspect the current category',
      no_cat_prefix:'Current category "', no_cat_suffix:'" has no inspectable accounts', cat_under:' ', confirm_suffix:' confirm',
      still_running:'Still running…', action_timeout:'Action timed out; refresh and verify whether it applied',
      delete_confirm_title:'Confirm delete',
      delete_body_prefix:'This will delete CPA Auth credential "', delete_body_suffix:'".\nThis cannot be undone. Continue?',
      no_enabled:'No enabled accounts available to disable', no_disabled:'No disabled accounts available to enable', no_actionable:'No actionable accounts',
      only_enabled_rows:'Only accounts currently listed as Enabled.',
      only_disabled_rows:'Only accounts currently listed as Disabled.',
      all_in_category:'Includes all accounts in the current category.',
      bulk_delete_api:'Calls CPA bulk-delete API (DELETE /auth-files, up to 50 per batch) and updates local results.\n',
      irreversible:'This cannot be undone.',
      bulk_patch_prefix:'Uses the CPA Management API to ', bulk_patch_suffix:' accounts and updates local results.\n',
      bulk_patch_note:'Note: CPA has no bulk enable/disable API, so PATCH is called one-by-one (~6 concurrent workers in the plugin). ',
      bulk_patch_slow:'Large account sets may be slower; progress is shown above.\n',
      bulk_patch_hint:'For faster cleanup, use Bulk delete (CPA supports multi-delete).',
      bulk_word:'Bulk ', current_category:'Current category: ', affected:'Affected accounts: ', units:'\n',
      will_bulk:'Will bulk-', continue_q:'Continue?',
      started_prefix:'Started: ', started_suffix:' item(s) (running in background; see status above)',
      no_export_suffix:'" has no exportable data', export_confirm:'Confirm bulk export',
      export_count:'Rows to export: ', export_rows:'\n\n',
      export_body:'Exports all accounts in the current category (not only the current page) as a JSON file.\n\n',
      no_export_filter:'No exportable data in the current filter',
      showing:'Showing ', per_page:' · per page ', prev:'Prev', next:'Next',
      inspect_running:'Inspection running', category_running:'Category inspection running', incremental_running:'Incremental inspection running',
      category_only_keep:' (current category only; keep other results)', incremental_only_keep:' (new accounts only; keep existing results)', bg_continue:' (continues in background)',
      timeout_recheck:' · timeout recheck ', recheck_workers:' · recheck workers ', workers_sep:' · workers ',
      category_stopped:'Category inspection stopped', incremental_stopped:'Incremental inspection stopped', stopped:'Stopped',
      this_run:', this run ', list_total:', list total ', accounts_word:' accounts',
      inspection_complete:'Inspection complete, ', category_complete:'Category pass complete: probed ', list_mid:', list total ', list_end:'',
      incremental_complete:'Incremental complete: newly probed ', persisted:' · persisted', last_fail:' · last action failed ', rows_word:'', rows_paren:')',
      apply_confirm_title:'Confirm apply suggested actions',
      apply_body_prefix:'Will asynchronously disable/enable/delete accounts with suggested actions across all results (',
      apply_body_suffix:' suggestion(s)).\nNote: this follows suggestions and is not limited by the category card filter above.\n\n',
      start_failed:'Start failed', suggested_running_prefix:'Suggested actions running in background: ', items_word:' item(s)',
      suggested_started:'Suggested actions started', bg_actions:'Background actions ', failed_sep:'; failed ',
      no_action_seq:'Server did not return action_seq; cannot confirm the action result',
      deleted_prefix:'Deleted: ', disabled_prefix:'Disabled: ', enabled_prefix:'Enabled: ', running_prefix:'Running: ',
      key_manual:'Using a manually entered Key',
      hero_autoban:'Use the switch to control realtime auto-ban. Quota exhausted / permission denied / 401 are auto-banned; quota bans restore after 24h by default, others need manual unban.',
      filter_count_prefix:'Current category: ', filter_count_mid:' (', filter_count_suffix:' rows)',
      action_failed_suffix:' failed',
      bulk_confirm_title_prefix:'Bulk ', bulk_confirm_title_suffix:' confirm',
      bulk_started_prefix:'Bulk ',
      no_cat_action_mid:'" — ',
      export_hint_prefix:'Current category: ',
      unban_running:'Unban in progress ', unban_fail_sep:' · failed ', unban_persist_sep:' · save failed: ',
      apply_progress:'Background actions ',
      persist_fail_sep:' · save failed: ',
      unban_progress_complete_fail:'Failed to save auto-ban state: ',
      hours_minutes_mid:'h ', hours_minutes_suffix:'m', minutes_suffix:'m',
      ban_unknown_reason:'Unknown reason',
      ban_reason_quota:'Quota exhausted', ban_reason_permission:'Permission denied', ban_reason_authfail:'Auth failed',
      restore_header_absolute:'Auto restore at upstream reset time',
      restore_header_relative:'Auto restore after Retry-After',
      restore_date_plus_fallback:'Auto restore after cooldown window',
      restore_local_plus_fallback:'Scheduled auto restore',
      ban_unban_filter_named_prefix:'Unban "', ban_unban_filter_named_suffix:'"',
      ban_filter_hint_all:'Click a card below to filter; unban current filter applies to all banned accounts when All is selected',
      ban_filter_current_prefix:'Current filter: ', ban_filter_current_mid:' · ', ban_filter_current_suffix:' total',
      ban_empty_filter_prefix:'No accounts in filter "', ban_empty_filter_suffix:'"',
      ban_need_key_load:'Enter the CPA Management Key to load auto-ban status',
      ban_not_loaded:'Not loaded', ban_running:'Running',
      pager_total_prefix:'', pager_total_suffix:' total', pager_page_mid:' · page ', pager_page_of:' / ', pager_page_suffix:'',
      unban_confirm_title:'Confirm unban', unban_confirm_body_prefix:'Re-enable account:\n',
      unban_failed:'Unban failed', unban_success_title:'Unban succeeded',
      unban_missing:'Account is no longer in the CPA auth list; local ban record cleared',
      unban_success_msg:'Unbanned and re-enabled',
      unban_filter_empty:'No accounts to unban in the current filter',
      unban_filter_confirm_title:'Confirm unban current filter',
      unban_filter_confirm_body_prefix:'Will unban filter "', unban_filter_confirm_body_mid:'" — ', unban_filter_confirm_body_suffix:' account(s).\nRuns in background; use Stop to cancel.',
      unban_in_progress:'Unbanning…', unban_start_failed:'Failed to start unban',
      unban_filter_started_prefix:'Filter unban started in background: ', unban_filter_started_suffix:'',
      unban_all_confirm_title:'Confirm unban all',
      unban_all_confirm_body:'Attempt to unban every account in the current ban pool.\nRuns in background; use Stop to cancel.',
      unban_all_started:'Unban-all started in background',
      notice_title:'Notice',
      apply_disable_line:'· Disable ', apply_enable_line:'· Enable ', apply_delete_line:'· Delete ',
      apply_body_counts_mid:'\n',
    }
  }

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
    const sel = document.getElementById('langSelect');
    if (sel) sel.value = lang;
    if (typeof classLabel !== 'undefined') {
      classLabel.healthy = t('class_healthy');
      classLabel.permission_denied = t('class_permission_denied');
      classLabel.quota_exhausted = t('class_quota_exhausted');
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
    reason = String(reason == null ? '' : reason).trim();
    if (!reason) return reason;
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
    // Formatted: list accounts failed
    for (const cat of catalogs) {
      const prefix = cat.list_accounts_failed_prefix;
      if (prefix && reason.indexOf(prefix) === 0) {
        return formatListAccountsFailed(reason.slice(prefix.length));
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
    return reason;
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
      if (hasManagementKey()) refresh();
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
    // Guard against pointerdown + click double fire.
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
    const fire = (ev) => {
      try {
        if (ev) {
          ev.preventDefault();
          ev.stopPropagation();
        }
      } catch (_) {}
      closeConfirm(value);
    };
    // pointerdown responds before click; click kept as fallback.
    el.onpointerdown = fire;
    el.onclick = fire;
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
      modal.onpointerdown = (ev) => {
        if (ev.target === modal) closeConfirm(false);
      };
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
        return { ok: false, error: hit.error || (act + ' failed'), report: hit };
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
          delete: act === 'delete'
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
          auth_indexes: indexes
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
  });
  // If a key is available, keep plugin storage in sync for next open.
  if (keyInput.value.trim()) persistManagementKey(keyInput.value);
  updateAuthState();
  const classLabel = {
    healthy: t('class_healthy'), permission_denied: t('class_permission_denied'), quota_exhausted: t('class_quota_exhausted'),
    reauth: t('class_reauth'), model_unavailable: t('class_model_unavailable'), probe_error: t('class_probe_error'), unknown: t('class_unknown')
  };
  const actionLabel = { keep: t('action_keep'), disable: t('action_disable'), enable: t('action_enable'), delete: t('action_delete') };
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
      all: t('class_all'),
      healthy: t('class_healthy'),
      permission_denied: t('class_permission_denied'),
      quota_exhausted: t('class_quota_exhausted'),
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
      let msg = t('apply_progress') + (snap.apply_done||0) + '/' + (snap.apply_total||0) + (snap.apply_current ? '：' + snap.apply_current : '');
      if ((snap.apply_failures || []).length) msg += t('failed_sep') + snap.apply_failures.length;
      setProgress(msg, true);
    } else if (snap.running) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const mode = scoped ? t('category_running') : (snap.incremental ? t('incremental_running') : t('inspect_running'));
      const extra = scoped ? t('category_only_keep') : (snap.incremental ? t('incremental_only_keep') : t('bg_continue'));
      let phase = '';
      if (snap.probe_phase === 'retry') {
        phase = t('timeout_recheck') + (snap.retry_done||0) + '/' + (snap.retry_total||0) + t('recheck_workers') + (snap.retry_workers||1);
      }
      setProgress(mode + ' ' + (snap.done||0) + '/' + (snap.total||0) + t('workers_sep') + (snap.workers||WORKERS_DEFAULT) + phase + extra, true);
    } else if (snap.stopped) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      const mode = scoped ? t('category_stopped') : (snap.incremental ? t('incremental_stopped') : t('stopped'));
      setProgress(mode + t('this_run') + (snap.done||0) + (snap.total ? '/' + snap.total : '') + t('list_total') + ((snap.results||[]).length) + t('accounts_word'), false);
    } else if ((snap.results||[]).length) {
      const scoped = Array.isArray(snap.classifications) && snap.classifications.length > 0;
      let msg = t('inspection_complete') + (snap.results||[]).length + t('accounts_word');
      if (scoped && (snap.done||0) >= 0 && snap.total != null) {
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
      completedErrors.push(...(snap.apply_failures || []));
    }
    if (snap.unban && !snap.unban.running) {
      if ((snap.unban.failures || []).length) {
        completedErrors.push(...(snap.unban.failures || []));
      } else if (snap.unban.persist_error) {
        completedErrors.push(t('unban_progress_complete_fail') + snap.unban.persist_error);
      }
    }
    if (completedErrors.length) {
      // Keep failures visible after an asynchronous job has finished.
      showErr(completedErrors.join('\n'));
    }
  }
  let pollTimer = null;
  const POLL_MS = 1200;
  // Full results are heavy at 1000+ accounts; keep light polls frequent, full list rarer while running.
  const LIVE_RESULTS_MS = 10000;
  const LIVE_RESULTS_IDLE_MS = 2400;
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
  function startPolling() {
    if (pollTimer != null) return;
    // Light polls omit results[] — progress only.
    pollTimer = setInterval(() => {
      if (confirmOpen) return; // modal open: keep UI responsive for cancel/ok
      refresh({ light: true });
    }, POLL_MS);
  }
  // Only poll while a server job is active; idle pages do not keep hitting /status.
  function syncPolling(snap) {
    const unbanRun = !!(snap && snap.unban && snap.unban.running);
    if (snap && (snap.running || snap.applying || unbanRun)) startPolling();
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
  async function syncFullResults(force, busyRunning) {
    if (fullResultsSyncing) return false;
    const minGap = busyRunning ? LIVE_RESULTS_MS : LIVE_RESULTS_IDLE_MS;
    if (!force && Date.now() - lastFullResultsAt < minGap) return false;
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
      const path = (light ? '/status?include_results=0' : '/status?include_results=1') + '&lang=' + encodeURIComponent(lang);
      const data = await api(path, { method: 'GET' });
      const busy = !!(data && (data.running || data.applying || (data.unban && data.unban.running)));
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
      // Avoid expensive table re-render under an open modal (cancel feels stuck).
      if (!confirmOpen) render();
      else {
        // Still keep progress text cheaply updated.
        try {
          const snap = state.snapshot || {};
          if (snap.running || snap.applying) {
            // progress only path inside render is heavy; update line directly
            if (snap.applying) {
              setProgress(t('apply_progress') + (snap.apply_done||0) + '/' + (snap.apply_total||0), true);
            } else if (snap.running) {
              setProgress(t('inspect_running') + ' ' + (snap.done||0) + '/' + (snap.total||0), true);
            }
          }
        } catch (_) {}
      }

      // Job just finished → pull full results once (list may have changed a lot).
      if (wasBusy && !busy) {
        await syncFullResults(true, false);
        if (hasManagementKey()) { try { await loadBans(); } catch (_) {} }
        return;
      }
      // Keep the account table live during inspection without sending the full
      // 1000+ row payload on every progress poll.
      if (light && data && data.results_gen != null) {
        const gen = Number(data.results_gen) || 0;
        if (gen && gen !== lastResultsGen) {
          // While inspecting, throttle full list pulls; still force when job ends (wasBusy path).
          const synced = await syncFullResults(!busy, !!(data && (data.running || data.applying || (data.unban && data.unban.running))));
          if (synced) lastResultsGen = gen;
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
  $('runBtn').onclick = () => startInspection(false);
  $('incrBtn').onclick = () => startInspection(true);
  if ($('filterRunBtn')) $('filterRunBtn').onclick = () => startInspection('filter');
  $('stopBtn').onclick = async () => {
    try { await api('/stop', { method: 'POST', body: '{}' }); await refresh(); }
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
      const result = await api('/apply', { method: 'POST', body: '{}' });
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

  function heroTextFor(tab) {
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
    const m = Math.floor((sec %% 3600) / 60);
    if (h >= 24 * 30) return t('ban_manual');
    if (h > 0) return h + t('hours_minutes_mid') + String(m).padStart(2, '0') + t('hours_minutes_suffix');
    return m + t('minutes_suffix');
  }
  function formatBanReason(code) {
    const c = String(code || '').trim().toLowerCase();
    if (!c) return t('ban_unknown_reason');
    if (c === 'subscription:free-usage-exhausted' || c.indexOf('free-usage-exhausted') >= 0) return t('ban_reason_quota');
    if (c === 'permission-denied' || c.indexOf('permission-denied') >= 0) return t('ban_reason_permission');
    if (c === 'unauthorized' || c === '401' || c.indexOf('unauthorized') >= 0) return t('ban_reason_authfail');
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
    if (c === 'permission-denied' || c.indexOf('permission-denied') >= 0) return 'permission';
    if (c === 'unauthorized' || c === '401' || c.indexOf('unauthorized') >= 0) return 'unauthorized';
    return 'other';
  }
  function banFilterLabel(f) {
    if (f === 'quota') return t('ban_quota');
    if (f === 'permission') return t('ban_permission');
    if (f === 'unauthorized') return t('ban_authfail');
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
      return '<tr>' +
        '<td class="col-name">' + esc(id) + '</td>' +
        '<td>' + esc(formatBanReason(b.error_code)) + '</td>' +
        '<td>' + esc(formatShanghaiTime(b.banned_at)) + '</td>' +
        '<td>' + esc(formatResetSource(b.reset_source, b.remaining_seconds)) + '</td>' +
        '<td>' + esc(formatRemain(b.remaining_seconds)) + '</td>' +
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
        ((banState.filter && banState.filter !== 'all') ? ('（' + banFilterLabel(banState.filter) + '）') : '') +
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
      let q = 0, p = 0, u = 0;
      banState.bans.forEach((b) => {
        const c = banCategoryOf(b);
        if (c === 'quota') q++;
        else if (c === 'permission') p++;
        else if (c === 'unauthorized') u++;
      });
      set('banCount', data.count != null ? data.count : banState.bans.length);
      set('banQuotaCount', data.quota_count != null ? data.quota_count : q);
      set('banPermissionCount', data.permission_count != null ? data.permission_count : p);
      set('banUnauthorizedCount', data.unauthorized_count != null ? data.unauthorized_count : u);
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
  }

  (function bindLang() {
    const sel = document.getElementById('langSelect');
    if (sel) {
      sel.value = lang;
      sel.addEventListener('change', () => setLang(sel.value));
    }
    applyStaticI18n();
  })();
</script>
</body>
</html>`, base)
	return []byte(html)
}
