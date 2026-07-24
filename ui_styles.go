package main

const uiCSS = `    :root { color-scheme: light; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:#f5f7fb; color:#0f172a; }
    .wrap { max-width: 1480px; margin: 0 auto; padding: 18px clamp(12px,2vw,24px) 28px; }
    .hero { display:flex; justify-content:space-between; gap:16px; flex-wrap:wrap; margin-bottom:14px; }
    .badge { display:inline-flex; align-items:center; height:22px; padding:0 8px; border-radius:999px; background:#eef2ff; color:#3730a3; font-size:11px; font-weight:700; }
    h1 { margin:6px 0 0; font-size:22px; line-height:30px; }
    .sub { margin:4px 0 0; color:#64748b; font-size:13px; }
    .controls { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    .sample-controls { margin-top:0; }
    label.ctl, button { height:34px; border-radius:8px; font-size:13px; }
    label.ctl { display:inline-flex; align-items:center; gap:8px; padding:0 10px; border:1px solid #dbe1e8; background:#fff; color:#475569; }
    label.ctl span { white-space:nowrap; }
    input[type=number] {
      width:56px; height:26px; border:1px solid #cbd5e1; border-radius:6px; padding:0 6px;
      background:#fff; color:#0f172a; -webkit-appearance:none; appearance:textfield;
    }
    label.ctl input[type=number] {
      width:64px; height:30px; border:0; border-radius:0; padding:0 2px;
      background:transparent; color:inherit; box-shadow:none; outline:none;
    }
    label.ctl:focus-within {
      border-color:#93c5fd; box-shadow:0 0 0 3px rgba(37,99,235,.10);
    }
    label.ctl input[type=number]::-webkit-outer-spin-button,
    label.ctl input[type=number]::-webkit-inner-spin-button { -webkit-appearance:none; margin:0; }
    button { padding:0 12px; border:1px solid #d1d5db; background:#fff; color:#334155; cursor:pointer; }
    button.primary { border-color:#2563eb; background:#2563eb; color:#fff; font-weight:700; }
    button.soft { border-color:#c7d2fe; background:#eef2ff; color:#3730a3; font-weight:650; }
    button.danger { border-color:#fecaca; background:#fef2f2; color:#b91c1c; font-weight:650; }
    button:disabled { opacity:.55; cursor:not-allowed; }
    .summary { display:grid; grid-template-columns:repeat(6,minmax(100px,1fr)); gap:10px; margin-bottom:12px; }
    .summary.ban-summary { grid-template-columns:repeat(5,minmax(0,1fr)); width:100%; min-width:0; }
    .card { background:#fff; border:1px solid #e2e8f0; border-radius:10px; padding:12px; box-shadow:0 1px 2px rgba(15,23,42,.04); cursor:pointer; min-width:0; }
    .card.active { outline:2px solid #2563eb; }
    .card .k { color:#64748b; font-size:12px; line-height:1.3; overflow-wrap:anywhere; word-break:break-word; }
    .card .v { margin-top:4px; font-size:22px; font-weight:750; }
    .bar { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:10px; align-items:center; }
    .actions-row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    .actions-row .hint { font-size:12px; color:var(--muted,#64748b); }
    .schedule-row {
      display:flex; align-items:center; gap:8px; flex-wrap:wrap; width:100%;
      margin:10px 0 12px; padding:10px 0; border-top:1px solid #e2e8f0; border-bottom:1px solid #e2e8f0;
    }
    .schedule-row .schedule-status { margin-left:auto; color:var(--muted,#64748b); font-size:12px; line-height:1.5; text-align:right; }
    .schedule-row select { height:26px; border:1px solid #cbd5e1; border-radius:6px; background:#fff; color:#0f172a; padding:0 6px; }
    .progress { min-height:20px; font-size:12px; color:#64748b; display:inline-flex; align-items:center; gap:6px; padding:4px 10px; border-radius:8px; max-width:100%; }
    .progress.live { color:#1d4ed8; font-weight:700; background:#dbeafe; border:1px solid #93c5fd; box-shadow:0 0 0 1px rgba(37,99,235,.08); }
    .progress.live::before { content:""; width:8px; height:8px; border-radius:50%; background:#2563eb; box-shadow:0 0 0 0 rgba(37,99,235,.55); animation:pulseDot 1.2s ease-out infinite; flex:0 0 auto; }
    @keyframes pulseDot { 0% { box-shadow:0 0 0 0 rgba(37,99,235,.45); } 70% { box-shadow:0 0 0 8px rgba(37,99,235,0); } 100% { box-shadow:0 0 0 0 rgba(37,99,235,0); } }
    tr.row-out { opacity:0; transform:translateX(8px); transition:opacity .28s ease, transform .28s ease; }
    tr.row-busy { opacity:.55; }
    .row-actions { display:flex; gap:6px; flex-wrap:wrap; align-items:center; }
    .row-actions button { height:28px; padding:0 8px; font-size:12px; }
    .toast-ok { color:#047857; font-size:12px; margin-top:6px; }
    .modal { position:fixed; inset:0; z-index:10050; display:flex; align-items:center; justify-content:center; background:rgba(15,23,42,.45); padding:16px; }
    .modal.hidden { display:none; }
    .modal-card { width:min(440px,100%); background:#fff; border-radius:12px; border:1px solid #e2e8f0; box-shadow:0 20px 40px rgba(15,23,42,.18); padding:18px 18px 14px; }
    .modal-title { font-size:16px; font-weight:700; color:#0f172a; margin-bottom:10px; }
    .modal-msg { font-size:13px; line-height:1.6; color:#334155; white-space:pre-wrap; margin-bottom:16px; }
    .modal-actions { display:flex; justify-content:flex-end; gap:8px; }
    .modal-actions button { min-width:76px; touch-action:manipulation; -webkit-tap-highlight-color:transparent; }
    .table-wrap { background:#fff; border:1px solid #e2e8f0; border-radius:10px; overflow:hidden; box-shadow:0 1px 2px rgba(15,23,42,.04); }
    .table-wrap .table-scroll { overflow:auto; -webkit-overflow-scrolling:touch; }
    table { width:100%; border-collapse:collapse; min-width:1100px; font-size:13px; table-layout:auto; }
    .table-wrap.account-pool { width:100%; min-width:0; }
    .table-wrap.account-pool .table-scroll { width:100%; min-height:0; }
    .table-wrap.account-pool .empty {
      min-height:140px; display:flex; align-items:center; justify-content:center; box-sizing:border-box;
    }
    /* 巡检 / 自动禁用账号池同一套尺寸 */
    .table-wrap.account-pool table,
    .table-wrap.account-pool table.inspect-table,
    .table-wrap.account-pool table.ban-table {
      width:100%; min-width:1100px; table-layout:auto; font-size:13px; border-collapse:collapse;
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
    th { padding:10px 12px; border-bottom:1px solid #e2e8f0; text-align:left; background:linear-gradient(180deg,#f8fafc 0%,#f1f5f9 100%); color:#475569; font-size:12px; white-space:nowrap; }
    td { padding:10px 12px; border-bottom:1px solid #f1f5f9; vertical-align:middle; }
    td.col-reason { vertical-align:top; word-break:break-word; overflow-wrap:anywhere; }
    th.col-status, td.col-status, th.col-result, td.col-result { white-space:nowrap; width:1%; min-width:88px; }
    th.col-http, td.col-http { white-space:nowrap; width:1%; min-width:56px; text-align:center; }
    th.col-model, td.col-model { white-space:nowrap; min-width:72px; }
    th.col-action, td.col-action { white-space:nowrap; min-width:72px; }
    th.col-ops, td.col-ops { white-space:nowrap; width:1%; min-width:120px; }
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
    .key-row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; width:100%; }
    .key-row input { width:min(360px,100%); height:34px; border:1px solid #cbd5e1; border-radius:8px; padding:0 10px; }
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
    .grok-inspection-page .key-row input {
      color:var(--text) !important;
      background:var(--surface-subtle, var(--surface)) !important;
      border-color:var(--input-border) !important;
      color-scheme: inherit;
    }
    .grok-inspection-page label.ctl input[type=number] {
      color:var(--text) !important;
      background:transparent !important;
      border-color:transparent !important;
      color-scheme: inherit;
    }
    html[data-grok-theme="dark"] .grok-inspection-page label.ctl {
      background:var(--surface) !important;
      border-color:var(--border) !important;
      color:var(--text) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page label.ctl input[type=number] {
      background:transparent !important;
      color:var(--text) !important;
      border-color:transparent !important;
    }
    .grok-inspection-page th { background:var(--surface-subtle) !important; color:var(--muted) !important; border-color:var(--border) !important; }
    .grok-inspection-page td { border-color:var(--border-subtle) !important; }
    .grok-inspection-page .pager { background:var(--surface-muted) !important; border-color:var(--border) !important; }
    .grok-inspection-page .empty { color:var(--muted) !important; }
    .grok-inspection-page .settings-row,
    .grok-inspection-page .actions-row { display:flex; gap:8px; flex-wrap:wrap; width:100%; }
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
    html[data-grok-theme="dark"] .grok-inspection-page label.ctl input[type=number] {
      background:transparent !important;
      border-color:transparent !important;
      color:var(--text) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .schedule-row {
      border-color: var(--border) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .schedule-row select {
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
      .grok-inspection-page .controls { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; width:100%; }
      .grok-inspection-page .key-row { grid-column:1 / -1; grid-row:1; width:100%; }
      .grok-inspection-page .key-row input { width:100%; min-width:0; height:42px; font-size:16px; }
      .grok-inspection-page .controls > label { width:100%; min-width:0; padding:0 8px; }
      .grok-inspection-page .controls > label:first-of-type { grid-column:1 / -1; grid-row:2; }
      .grok-inspection-page .controls > label:nth-of-type(2) { grid-column:1; grid-row:3; }
      .grok-inspection-page .controls > label:nth-of-type(3) { grid-column:2; grid-row:3; }
      .grok-inspection-page input[type=number] { flex:1; width:100%; min-width:0; }
      .grok-inspection-page .controls > #stopBtn { grid-column:1; grid-row:4; width:100%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #runBtn { grid-column:2; grid-row:4; width:100%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #incrBtn { grid-column:1 / -1; grid-row:5; width:100%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #applyBtn { grid-column:1 / -1; grid-row:6; width:100%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #filterRunBtn { grid-column:1 / -1; grid-row:7; width:100%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .sample-controls { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; width:100%; margin-top:8px; }
      .grok-inspection-page .sample-controls > label { width:100%; min-width:0; padding:0 8px; }
      .grok-inspection-page .sample-controls > #sampleBtn { grid-column:1 / -1; width:100%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .summary,
      .grok-inspection-page .summary.ban-summary {
        grid-template-columns:repeat(2,minmax(0,1fr)) !important;
        gap:8px; width:100%; min-width:0;
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
        margin-top:8px; width:100%; gap:8px;
      }
      .grok-inspection-page .actions-row > button { width:100%; min-width:0; padding:0 8px; }
      .grok-inspection-page .actions-row .hint {
        grid-column:1 / -1; line-height:1.5; overflow-wrap:anywhere;
      }
      .grok-inspection-page .schedule-row { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; }
      .grok-inspection-page .schedule-row > label,
      .grok-inspection-page .schedule-row > button { width:100%; min-width:0; }
      .grok-inspection-page .schedule-row .schedule-status {
        grid-column:1 / -1; margin-left:0; text-align:left; overflow-wrap:anywhere;
      }
      .grok-inspection-page .pager { align-items:stretch; }
      .grok-inspection-page .pager > div { width:100%; }
      .grok-inspection-page .pager > div:last-child { justify-content:space-between; }
      .grok-inspection-page .tabs {
        width:100% !important; max-width:100% !important;
        display:grid !important; grid-template-columns:1fr 1fr !important; gap:6px;
        box-sizing:border-box;
      }
      .grok-inspection-page .tab,
      .grok-inspection-page button.tab {
        flex:none !important; width:100% !important; min-width:0 !important;
        max-width:none !important; align-items:flex-start !important;
        box-sizing:border-box;
      }
      .grok-inspection-page .table-wrap.account-pool table,
      .grok-inspection-page .table-wrap.account-pool table.inspect-table,
      .grok-inspection-page .table-wrap.account-pool table.ban-table {
        min-width:720px !important;
      }
      .grok-inspection-page .table-wrap.account-pool { border-radius:10px; width:100%; }
      .grok-inspection-page .table-wrap.account-pool .empty { min-height:120px; }
      .grok-inspection-page .table-wrap.account-pool code {
        font:inherit; background:transparent; border:0; padding:0; color:inherit;
      }
    }
    .grok-inspection-page .tabs {
      display:inline-flex; gap:6px; flex-wrap:wrap; margin:0 0 14px; padding:3px;
      background:var(--surface,#fff); border:1px solid var(--border,#e2e8f0); border-radius:10px;
      width:fit-content; max-width:100%; box-sizing:border-box;
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
    .slider:before { position:absolute; content:""; height:20px; width:20px; left:3px; top:3px; background:#fff; transition:.18s; border-radius:50%; box-shadow:0 1px 2px rgba(15,23,42,.2); }
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
      .grok-inspection-page .tabs { width:fit-content; max-width:100%; }
      .grok-inspection-page .tab,
      .grok-inspection-page button.tab { flex:0 0 auto !important; width:auto !important; }
      .autoban-split, .settings-grid { grid-template-columns:1fr; }
    }
    /* 手机端最终覆盖：保证 tabs 铺满，且两个账号池同宽 */
    @media (max-width:760px) {
      .grok-inspection-page .tabs {
        width:100% !important; max-width:100% !important;
        display:grid !important; grid-template-columns:1fr 1fr !important; gap:6px !important;
        box-sizing:border-box !important;
      }
      .grok-inspection-page .tab,
      .grok-inspection-page button.tab {
        flex:none !important; width:100% !important; min-width:0 !important;
        max-width:none !important; box-sizing:border-box !important;
      }
      .grok-inspection-page .table-wrap.account-pool {
        width:100% !important; min-width:0 !important;
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
`
