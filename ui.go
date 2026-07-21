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
        <h1>Grok 账号巡检</h1>
        <p class="sub" id="heroSub">「开始巡检」清空并重测全部；「增量巡检」只测新增账号；「巡检当前分类」只重测所选分类（需先点分类卡片）；「批量操作」只作用于当前筛选；结果会自动保存。</p>
      </div>
    </div>
    <div class="controls shared-key" id="keyRow">
      <div class="key-row" style="flex:1;min-width:min(360px,100%%)">
        <input id="managementKey" type="password" autocomplete="current-password" placeholder="CPA Management Key（可自动读取管理面板）">
        <span class="hint" id="keyHint"></span>
      </div>
    </div>
    <div class="tabs" role="tablist" aria-label="功能页签">
      <button class="tab active" type="button" data-tab="inspect" id="tabInspect" aria-selected="true" role="tab"><span class="tab-title">账号巡检</span><span class="tab-desc">批量探测 · 建议操作</span></button>
      <button class="tab" type="button" data-tab="autoban" id="tabAutoban" aria-selected="false" role="tab"><span class="tab-title">实时自动禁用</span><span class="tab-desc">请求拦截 · 定时恢复</span></button>

    </div>
    <section class="panel active" id="panel-inspect">
    <div class="controls">
      <label class="ctl">并发 <input id="workers" type="number" min="1" max="16" step="1" value="6" title="1-16 的整数"></label>
      <label class="ctl"><input id="includeDisabled" type="checkbox"> 包含已禁用</label>
      <label class="ctl"><input id="onlyDisabled" type="checkbox"> 仅巡检已禁用</label>
      <button id="stopBtn" disabled>停止</button>
      <button id="applyBtn" class="soft" disabled>执行建议操作</button>
      <button id="incrBtn" class="soft" disabled title="只检测 Auth 中相对上次结果新增的账号">增量巡检</button>
      <button id="filterRunBtn" class="soft" disabled title="只重新探测当前卡片筛选分类下的账号，保留其他结果">巡检当前分类</button>
      <button id="runBtn" class="primary">开始巡检</button>
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

      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="inspect-table">
            <thead>
              <tr>
                <th class="col-name">账号</th><th class="col-status">当前状态</th><th class="col-result">检测结果</th><th class="col-http">HTTP</th><th class="col-model">模型</th><th class="col-action">建议</th><th class="col-reason">原因</th><th class="col-ops">操作</th>
              </tr>
            </thead>
            <tbody id="rows"></tbody>
          </table>
        </div>
        <div id="empty" class="empty">请输入 CPA Management Key 后加载巡检状态</div>
        <div id="pager" class="pager"></div>
      </div>
    </section>

    <section class="panel" id="panel-autoban">
      <div class="module-bar">
        <div>
          <h2>实时自动禁用</h2>
        </div>
        <div class="switch-row">
          <label class="switch" title="开启后实时拦截并禁用">
            <input id="banEnabledToggle" type="checkbox">
            <span class="slider"></span>
          </label>
          <span id="banEnabledPill" class="status-pill off">已关闭</span>
          <span class="hint" id="banEnabledHint" class="hint">开关会立即生效并保存</span>
        </div>
      </div>
      <div class="controls" style="margin-bottom:12px">
        <button id="banRefreshBtn" class="soft" type="button">刷新状态</button>
        <button id="banUnbanFilterBtn" class="soft" type="button" disabled>解禁当前分类</button>
        <button id="banUnbanAllBtn" class="danger" type="button">全部解禁</button>
        <span class="hint" id="banFilterHint" class="hint">点击下方卡片筛选分类</span>
      </div>
      <div id="banSummary" class="summary ban-summary">
        <div class="card active" data-ban-filter="all"><div class="k">全部</div><div class="v" id="banCount">0</div></div>
        <div class="card" data-ban-filter="quota"><div class="k">额度用尽</div><div class="v" id="banQuotaCount">0</div></div>
        <div class="card" data-ban-filter="permission"><div class="k">权限拒绝</div><div class="v" id="banPermissionCount">0</div></div>
        <div class="card" data-ban-filter="unauthorized"><div class="k">401 认证失败</div><div class="v" id="banUnauthorizedCount">0</div></div>
      </div>
      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="ban-table">
            <thead>
              <tr>
                <th class="col-name">账号</th><th>禁用原因</th><th>禁用时间</th><th>恢复方式</th><th>剩余</th><th class="col-ops">操作</th>
              </tr>
            </thead>
            <tbody id="banRows"></tbody>
          </table>
        </div>
        <div id="banEmpty" class="empty">加载中…</div>
        <div id="banPager" class="pager"></div>
      </div>
      <pre id="banError" class="err" style="margin-top:10px;font-size:12px;white-space:pre-wrap"></pre>
    </section>
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

    </div>
  <script>
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
      hint.textContent = '已从管理面板自动读取 Key（无需手填）';
      keyInput.placeholder = '已自动填充（可改）';
    } else if (hasManagementKey() && keySource === 'plugin') {
      hint.textContent = '已使用本插件本地保存的 Key';
    } else if (hasManagementKey()) {
      hint.textContent = '已使用手动填写的 Key';
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
      titleEl.textContent = title || '确认操作';
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
  // If a key is available, keep plugin storage in sync for next open.
  if (keyInput.value.trim()) persistManagementKey(keyInput.value);
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
      '<div class="pager-meta">显示 ' + from + '-' + to + ' / ' + rows.length +
      ' · 每页 <select id="pageSize">' +
      [20,50,100].map((n) => '<option value="' + n + '"' + (state.pageSize===n?' selected':'') + '>' + n + '</option>').join('') +
      '</select></div>' +
      '<div style="display:flex;gap:8px;align-items:center">' +
      '<button id="prev"' + (state.page<=1?' disabled':'') + '>上一页</button>' +
      '<span class="pager-meta">' + state.page + ' / ' + totalPages + '</span>' +
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
    $('stopBtn').disabled = !hasManagementKey() || !(snap.running || snap.applying || (snap.unban && snap.unban.running));
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
    if (!hasManagementKey()) {
      setProgress('请输入 CPA Management Key 后加载巡检状态', false);
    } else if (snap.unban && snap.unban.running) {
      let msg = '解禁进行中 ' + (snap.unban.done||0) + '/' + (snap.unban.total||0) + (snap.unban.current ? ' · ' + snap.unban.current : '');
      if ((snap.unban.failures || []).length) msg += ' · 失败 ' + snap.unban.failures.length; if (snap.unban.persist_error) msg += ' · 保存失败: ' + snap.unban.persist_error;
      setProgress(msg, true);
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
      if (snap.persist_error) msg += ' · 保存失败: ' + snap.persist_error; if (snap.store_path) msg += ' · 已落盘';
      if ((snap.apply_failures || []).length) msg += ' · 上次操作失败 ' + snap.apply_failures.length + ' 条';
      setProgress(msg, false);
    } else {
      setProgress('等待开始', false);
    }
    const completedErrors = [];
    if ((snap.apply_failures || []).length && !snap.applying) {
      completedErrors.push(...(snap.apply_failures || []));
    }
    if (snap.unban && !snap.unban.running) {
      if ((snap.unban.failures || []).length) {
        completedErrors.push(...(snap.unban.failures || []));
      } else if (snap.unban.persist_error) {
        completedErrors.push('保存自动禁用状态失败: ' + snap.unban.persist_error);
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
      const path = light ? '/status?include_results=0' : '/status?include_results=1';
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
              setProgress('后台执行操作 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0), true);
            } else if (snap.running) {
              setProgress('巡检中 ' + (snap.done||0) + '/' + (snap.total||0), true);
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
      '执行建议操作确认',
      '将对全部结果中「有建议动作」的账号异步执行（共 ' + actionCount + ' 条建议）：\n' +
      '· 禁用 ' + disableN + ' 个\n' +
      '· 启用 ' + enableN + ' 个\n' +
      '· 删除 ' + deleteN + ' 个\n\n' +
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
  wireExclusive();

  const heroText = {
    inspect: '「开始巡检」清空并重测全部；「增量巡检」只测新增账号；「巡检当前分类」只重测所选分类（需先点分类卡片）；「批量操作」只作用于当前筛选；结果会自动保存。',
    autoban: '右侧开关控制实时自动禁用。命中额度用尽 / 权限拒绝 / 401 时自动禁用账号；额度用尽默认 24 小时后恢复，其余需手动解禁。',
  };
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
    if (sub) sub.textContent = heroText[name] || heroText.inspect;
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
    if (h >= 24 * 30) return '需手动解禁';
    if (h > 0) return h + ' 小时 ' + String(m).padStart(2, '0') + ' 分';
    return m + ' 分';
  }
  function formatBanReason(code) {
    const c = String(code || '').trim().toLowerCase();
    if (!c) return '未知原因';
    if (c === 'subscription:free-usage-exhausted' || c.indexOf('free-usage-exhausted') >= 0) return '额度用尽';
    if (c === 'permission-denied' || c.indexOf('permission-denied') >= 0) return '权限被拒绝';
    if (c === 'unauthorized' || c === '401' || c.indexOf('unauthorized') >= 0) return '认证失败';
    return code;
  }
  function formatResetSource(source, remainingSec) {
    const s = String(source || '').trim().toLowerCase();
    const sec = Number(remainingSec || 0);
    if (s === 'manual_unban' || (Number.isFinite(sec) && sec > 24 * 30 * 3600)) return '需手动解禁';
    if (s === 'header_absolute') return '按上游重置时间自动恢复';
    if (s === 'header_relative') return '按 Retry-After 自动恢复';
    if (s === 'date_plus_fallback') return '冷却窗口自动恢复';
    if (s === 'local_plus_fallback') return '定时自动恢复';
    if (!s) return '定时自动恢复';
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
    if (f === 'quota') return '额度用尽';
    if (f === 'permission') return '权限拒绝';
    if (f === 'unauthorized') return '401 认证失败';
    return '全部';
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
        ? ('解禁当前分类' + (n ? ' (' + n + ')' : ''))
        : ('解禁「' + banFilterLabel(f) + '」' + (n ? ' (' + n + ')' : ''));
    }
    const hint = document.getElementById('banFilterHint');
    if (hint) {
      hint.textContent = f === 'all'
        ? '点击下方卡片筛选分类；解禁当前分类作用于全部禁用账号'
        : ('当前筛选：' + banFilterLabel(f) + ' · 共 ' + filtered.length + ' 个');
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
        '<td><button type="button" data-unban="' + esc(id).replace(/"/g, '&quot;') + '">解禁</button></td>' +
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
            ? ('当前分类「' + banFilterLabel(banState.filter) + '」没有账号')
            : '当前没有自动禁用中的账号')
          : '请输入 CPA Management Key 后加载自动禁用状态';
      }
    }
    if (pager) {
      pager.innerHTML =
        '<div class="pager-meta pager-meta-row">共 ' + list.length + ' 个' +
        ((banState.filter && banState.filter !== 'all') ? ('（' + banFilterLabel(banState.filter) + '）') : '') +
        ' · 第 ' + banState.page + ' / ' + pages + ' 页' +
        ' · 每页 <select id="banPageSize">' +
        [20,50,100].map((n) => '<option value="' + n + '"' + (size===n?' selected':'') + '>' + n + '</option>').join('') +
        '</select></div>' +
        '<div style="display:flex;gap:8px;align-items:center">' +
        '<button id="banPrev"' + (banState.page<=1?' disabled':'') + '>上一页</button>' +
        '<button id="banNext"' + (banState.page>=pages?' disabled':'') + '>下一页</button></div>';
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
      if (pill) { pill.textContent = '未加载'; pill.className = 'status-pill off'; }
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
        pill.textContent = on ? '运行中' : '已关闭';
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
      const ok = await confirmDialog('确认解禁', '将重新启用账号：\n' + id);
      if (!ok) return;
      const raw = await api('/unban', { method: 'POST', body: JSON.stringify({ auth_id: id }) });
      const data = (raw && raw.result && typeof raw.result === 'object') ? raw.result : (raw || {});
      if (data && data.ok === false) throw new Error(data.error || '解禁失败');
      const msg = data && data.missing
        ? '账号已不在 CPA 认证列表中，已清除本插件禁用记录'
        : '已解禁并重新启用';
      await confirmDialog('解禁成功', msg, { showCancel: false });
      await loadBans();
    } catch (e) {
      try { await confirmDialog('解禁失败', String(e.message || e), { showCancel: false }); } catch (_) {}
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
        await confirmDialog('提示', '当前分类没有可解禁账号', { showCancel: false });
        return;
      }
      const label = banFilterLabel(banState.filter || 'all');
      const ok = await confirmDialog(
        '确认解禁当前分类',
        '将解禁「' + label + '」下的 ' + list.length + ' 个账号。\n后台异步执行，可用停止按钮中止。'
      );
      if (!ok) return;
      if (btn) { btn.disabled = true; btn.textContent = '解禁中…'; }
      const ids = list.map((b) => String(b.auth_id || '').trim()).filter(Boolean);
      const body = (banState.filter && banState.filter !== 'all')
        ? { category: banState.filter, auth_ids: ids }
        : { auth_ids: ids };
      const data = await api('/unban-all', { method: 'POST', body: JSON.stringify(body) });
      if (data && data.ok === false) throw new Error(data.error || '启动解禁失败');
      showOk('分类解禁已在后台执行：共 ' + ids.length + ' 个');
      startPolling();
      await refresh({ light: true });
      await loadBans();
    } catch (e) {
      try { await confirmDialog('解禁失败', String(e.message || e), { showCancel: false }); } catch (_) {}
      if (errEl) errEl.textContent = String(e.message || e);
    } finally {
      unbanCurrentFilter._busy = false;
      syncBanFilterUI();
    }
  }
  async function unbanAll() {
    const ok = await confirmDialog('确认全部解禁', '将尝试解禁当前禁用池中的全部账号。\n后台异步执行，可用停止按钮中止。');
    if (!ok) return;
    try {
      const data = await api('/unban-all', { method: 'POST', body: '{}' });
      if (data && data.ok === false) throw new Error(data.error || '启动解禁失败');
      showOk('全部解禁已在后台执行');
      startPolling();
      await refresh({ light: true });
      await loadBans();
    } catch (e) {
      await confirmDialog('提示', String(e.message || e));
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
        pill.textContent = enabled ? '运行中' : '已关闭';
        pill.className = 'status-pill ' + (enabled ? 'on' : 'off');
      }
      const label = document.getElementById('banEnabledLabel');
      if (label) label.textContent = enabled ? '运行中' : '已关闭';
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
</script>
</body>
</html>`, base)
	return []byte(html)
}
