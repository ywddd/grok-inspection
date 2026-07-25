package main

const uiCSS = `
    :root {
          color-scheme: light;
          --bg: #f5f7fb;
          --page-bg: #f5f7fb;
          --surface: #ffffff;
          --surface-muted: #fbfdff;
          --surface-subtle: #f8fafc;
          --surface-2: #f8fafc;
          --line: #e2e8f0;
          --line-subtle: #f1f5f9;
          --line-strong: #cbd5e1;
          --input-border: #cbd5e1;
          --text: #0f172a;
          --muted: #64748b;
          --muted-2: #94a3b8;
          --primary: #2563eb;
          --primary-hover: #1d4ed8;
          --primary-soft: #eef2ff;
          --green: #047857;
          --green-soft: #dcfce7;
          --amber: #b45309;
          --red: #b91c1c;
          --red-soft: #fef2f2;
          --purple: #3730a3;
          --purple-soft: #eef2ff;
          --shadow: 0 1px 2px rgba(15, 23, 42, .05);
        }
        * { box-sizing: border-box; }
        [hidden] { display: none !important; }
        html, body { margin: 0; min-height: 0; min-width: 0; max-width: 100%; }
        body {
          background: var(--bg) !important; color: var(--text) !important;
          font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
          font-size: 14px; line-height: 1.45; letter-spacing: 0; overflow-x: hidden;
        }
        button, input, select { font: inherit; color: inherit; }
        button { cursor: pointer; }
        svg, svg.ico { width: 16px; height: 16px; stroke-width: 1.8; flex: 0 0 auto; }
        .grok-inspection-page {
          width: min(1480px, calc(100% - 36px)); margin: 0 auto; padding: 18px 0 42px;
          color: var(--text); min-width: 0; max-width: 100%;
        }
        .sr-only { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip:rect(0,0,0,0); white-space:nowrap; border:0; }

        .page-head { display:flex; align-items:flex-start; justify-content:space-between; gap:18px; margin-bottom:14px; min-width:0; }
        .page-head-main { min-width:0; }
        .eyebrow, .badge.eyebrow, .eyebrow.badge {
          display:inline-flex; align-items:center; min-height:24px; margin-bottom:5px; padding:3px 8px;
          border-radius:6px; color:#3730a3; background:#eef2ff; font-size:11px; font-weight:700;
        }
        .title-row { display:flex; align-items:center; gap:7px; min-width:0; }
        h1 { margin:0; font-size:23px; line-height:1.25; letter-spacing:0; }
        .help-wrap { position:relative; flex:0 0 auto; }
        .icon-btn, .help-btn {
          width:34px; height:34px; display:inline-grid; place-items:center; border:1px solid transparent;
          border-radius:7px; background:transparent; color:#64748b; padding:0;
        }
        .icon-btn:hover, .help-btn:hover { background:var(--surface-2); border-color:var(--line); color:var(--text); }
        .title-row .help-btn {
          width:28px; height:28px; border-radius:7px; color:var(--muted-2); border:1px solid transparent;
        }
        .title-row .help-btn .ico { width:15px; height:15px; }
        .title-row .help-btn:hover,
        .title-row .help-btn[aria-expanded="true"] {
          background:var(--primary-soft); border-color:transparent; color:var(--primary);
        }
        .help-popover {
          position:absolute; z-index:100; top:-4px; left:calc(100% + 10px); right:auto; width:min(380px, calc(100vw - 48px));
          padding:12px 14px; border:1px solid var(--line-strong); border-radius:8px; background:var(--surface);
          box-shadow:0 12px 32px rgba(15,23,42,.14); color:var(--muted); font-size:13px;
          opacity:0; visibility:hidden; transform:translateY(-4px); transition:.15s ease;
          max-height:min(50vh, 280px); overflow-y:auto;
        }
        .help-popover[hidden] { display:none !important; }
        .help-popover.open { opacity:1; visibility:visible; transform:translateY(0); display:block; }
        .help-body p { margin:0; }
        .help-body p + p { margin-top:8px; }
        .help-body strong, .help-popover strong { color:var(--text); }
        .head-actions { display:flex; align-items:center; gap:8px; padding-top:25px; flex:0 0 auto; }
        .lang-ctl { display:inline-flex; align-items:center; margin:0; }
        .select-compact, .grok-inspection-page select, .pager select {
          height:34px; padding:0 28px 0 10px; border:1px solid var(--line-strong); border-radius:7px;
          background:var(--surface); color:var(--text); font-size:12px; color-scheme: inherit;
        }
        .panel, #panel-autoban, .autoban-control {
          border:1px solid var(--line); border-radius:8px; background:var(--surface); box-shadow:var(--shadow);
        }
        .controls-panel { overflow:hidden; }
        .access-row {
          min-height:48px; display:flex; align-items:center; gap:12px; padding:8px 12px;
          border-bottom:1px solid var(--line); min-width:0;
        }
        #managementKey.access-value, .access-row .access-value#managementKey, .access-value {
          width:min(360px,100%); min-width:0; flex:1 1 auto; height:32px; display:flex; align-items:center;
          padding:0 10px; border:1px solid var(--line); border-radius:7px; background:var(--surface-2);
          font-family:ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; color:#475569;
        }
        .key-state, #keyHint { display:inline-flex; align-items:center; gap:6px; color:var(--muted); font-weight:650; min-width:0; }
        .access-row #keyHint, .access-row #keyHint.key-state, .access-row .key-state {
          font-size:inherit; line-height:inherit;
        }
        #keyHint.ok, .key-state.ok { color:var(--green); }

        .mode-tabs, .grok-inspection-page .tabs {
          display:flex; align-items:stretch; padding:10px 12px 0; gap:4px; min-width:0; max-width:100%;
        }
        .tab, .mode-tab, button.tab, button.mode-tab {
          min-height:48px; min-width:0; max-width:100%; display:flex; align-items:center; gap:9px;
          padding:8px 14px; border:1px solid transparent; border-radius:7px 7px 0 0; background:transparent;
          color:var(--muted); font:inherit; font-weight:680; cursor:pointer; box-shadow:none;
          -webkit-appearance:none; appearance:none;
        }
        .tab > span, .mode-tab > span { min-width:0; flex:1 1 auto; overflow:hidden; text-align:left; }
        .tab .tab-title, .mode-tab .tab-title {
          display:block; font-size:14px; line-height:20px; overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
        }
        .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small {
          display:block; margin-top:2px; color:var(--muted-2); font-size:11px; line-height:16px; font-weight:520;
          overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
        }
        .grok-inspection-page .tab.active,
        .grok-inspection-page .mode-tab.active,
        .grok-inspection-page button.tab.active,
        .grok-inspection-page button.mode-tab.active {
          background:#2563eb !important; color:#fff !important; border-color:#2563eb !important;
          border-bottom-color:#2563eb !important; box-shadow:0 1px 2px rgba(37,99,235,.25) !important;
        }
        .grok-inspection-page .tab.active .tab-title,
        .grok-inspection-page .mode-tab.active .tab-title,
        .grok-inspection-page .tab.active .tab-desc,
        .grok-inspection-page .mode-tab.active .tab-desc,
        .grok-inspection-page .tab.active small,
        .grok-inspection-page .mode-tab.active small { color:#fff !important; opacity:1; }
        .tab:focus-visible, .mode-tab:focus-visible { outline:2px solid var(--primary); outline-offset:-2px; }

        .toolbar {
          display:grid; grid-template-columns:minmax(0,1fr) auto; gap:10px; padding:12px;
          border-top:1px solid var(--line); min-width:0;
        }
        .toolbar-main, .toolbar-actions, .sampling-controls, .bulk-actions, .result-tools, .autoban-action-buttons {
          display:flex; align-items:center; flex-wrap:wrap; gap:8px; min-width:0;
        }
        .schedule-controls {
          display:flex; align-items:center; flex-wrap:nowrap; gap:6px; min-width:0; overflow:hidden;
        }
        .schedule-controls .field, .schedule-controls .check-control {
          flex:0 1 auto; min-width:0; padding:0 8px; gap:6px;
        }
        .schedule-controls .field label, .schedule-controls .check-control span {
          overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:9.5em;
        }
        .schedule-controls .field.wide label { max-width:4.5em; }
        .schedule-controls .field input { width:36px; }
        .schedule-controls .field select { max-width:5.5em; }
        .schedule-row > .btn, .schedule-row #scheduleSaveBtn { flex:0 0 auto; white-space:nowrap; }
        .schedule-status, #scheduleStatus {
          overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:12em;
        }
        .field, .check-control {
          min-height:34px; display:inline-flex; align-items:center; gap:8px; padding:0 10px;
          border:1px solid var(--line); border-radius:7px; background:var(--surface); color:#475569; min-width:0;
        }
        .field label { color:var(--muted); font-size:12px; white-space:nowrap; }
        .field input[type="number"] {
          width:42px; min-width:0; padding:0; border:0; outline:0; background:transparent; text-align:center; font-weight:650;
        }
        .sampling-controls .field { justify-content:flex-start; }
        .sampling-controls .field input[type="number"] {
          width:52px; flex:0 0 52px; text-align:center;
        }
        .field select { border:0; background:transparent; outline:0; min-width:0; height:28px; padding:0 4px; }
        .check-control input { width:14px; height:14px; margin:0; accent-color:var(--primary); }
        .check-control span { white-space:nowrap; }
        .btn, .grok-inspection-page button.btn {
          min-height:34px; display:inline-flex; align-items:center; justify-content:center; gap:7px; padding:0 12px;
          border:1px solid #d1d5db; border-radius:7px; background:#fff; color:#334155;
          font-weight:650; white-space:nowrap;
        }
        .btn:hover:not(.primary):not(.soft):not(.danger) { border-color:#cbd5e1; background:var(--surface-2); }
        /* Specificity must beat .grok-inspection-page button.btn (v0.1.16 palette). */
        .grok-inspection-page .btn.primary,
        .grok-inspection-page button.primary {
          border-color:#2563eb !important; background:#2563eb !important; color:#fff !important; font-weight:700;
        }
        .grok-inspection-page .btn.primary:hover,
        .grok-inspection-page button.primary:hover {
          background:#1d4ed8 !important; border-color:#1d4ed8 !important;
        }
        html:not([data-grok-theme="dark"]) .grok-inspection-page .btn.soft,
        html:not([data-grok-theme="dark"]) .grok-inspection-page button.soft {
          border-color:#c7d2fe !important; background:#eef2ff !important; color:#3730a3 !important; font-weight:650;
        }
        html:not([data-grok-theme="dark"]) .grok-inspection-page .btn.danger,
        html:not([data-grok-theme="dark"]) .grok-inspection-page button.danger {
          border-color:#fecaca !important; background:#fef2f2 !important; color:#b91c1c !important; font-weight:650;
        }
        .btn[disabled], button:disabled { opacity:.55; cursor:not-allowed; }
        .sampling-row {
          display:flex; align-items:center; justify-content:space-between; gap:12px; padding:10px 12px;
          border-top:1px solid var(--line); background:var(--surface-muted); min-width:0;
        }
        .section-label {
          display:inline-flex; align-items:center; gap:7px; color:#475569; font-size:12px; font-weight:720; white-space:nowrap;
        }
        .schedule-row {
          display:grid; grid-template-columns:auto minmax(0,1fr) auto; align-items:center; gap:8px;
          padding:10px 12px; border-top:1px solid var(--line); min-width:0;
        }
        .schedule-status, #scheduleStatus {
          display:inline-flex; align-items:center; gap:6px; color:var(--muted); font-size:12px; font-weight:650;
          min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:11em;
        }
        .status-dot { width:7px; height:7px; border-radius:50%; background:#cbd5e1; flex:0 0 auto; }

        .autoban-control { margin-top:12px; overflow:hidden; }
        .autoban-bar {
          min-height:58px; display:flex; align-items:center; justify-content:space-between; gap:16px;
          padding:11px 12px; border-bottom:1px solid var(--line); min-width:0;
        }
        .autoban-heading { display:flex; align-items:center; gap:9px; min-width:0; }
        .autoban-heading-icon {
          width:34px; height:34px; display:grid; place-items:center; flex:0 0 auto; border-radius:7px;
          color:#3730a3; background:#eef2ff;
        }
        .autoban-heading-copy { min-width:0; }
        .autoban-heading-copy strong, .autoban-heading strong { display:block; font-size:15px; line-height:1.25; }
        .autoban-heading-copy small, .autoban-heading small, #banEnabledHint {
          display:block; margin-top:2px; color:var(--muted); font-size:11px; overflow-wrap:break-word; word-break:normal;
        }
        .autoban-switch-row, .switch-row { display:flex; align-items:center; gap:9px; flex:0 0 auto; }
        .switch, .toggle { position:relative; width:42px; height:24px; flex:0 0 auto; display:inline-block; }
        .switch input, .toggle input { position:absolute; width:1px; height:1px; opacity:0; pointer-events:none; }
        .slider, .toggle-track {
          position:absolute; inset:0; border-radius:12px; background:#cbd5e1; transition:.16s ease; cursor:pointer;
        }
        .slider:before, .toggle-track::after {
          content:''; position:absolute; top:3px; left:3px; width:18px; height:18px; border-radius:50%;
          background:#fff; box-shadow:0 1px 3px rgba(15,23,42,.28); transition:.16s ease;
        }
        .switch input:checked + .slider, .toggle input:checked + .toggle-track { background:#16a34a; }
        .switch input:checked + .slider:before, .toggle input:checked + .toggle-track::after { transform:translateX(18px); }
        .status-pill {
          min-height:25px; display:inline-flex; align-items:center; padding:3px 8px; border-radius:6px;
          color:#166534; background:#dcfce7; font-size:11px; font-weight:730; white-space:nowrap;
        }
        .status-pill.off { background:#fee2e2; color:#991b1b; }
        .status-pill.on { background:#dcfce7; color:#166534; }
        .autoban-actions {
          display:flex; align-items:center; justify-content:space-between; gap:12px; padding:10px 12px; flex-wrap:wrap; min-width:0;
        }
        .autoban-pool-status, #banFilterHint {
          color:var(--muted); font-size:12px; max-width:100%; overflow-wrap:break-word; word-break:normal; white-space:normal;
        }
        .ban-unsynced-banner { margin:8px 0 0; color:var(--amber); font-size:12px; }

        .summary, .overview { display:grid; gap:8px; margin-top:12px; width:100%; min-width:0; }
        .summary, .overview.inspection-summary { grid-template-columns:repeat(7, minmax(0, 1fr)); }
        .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns:repeat(6, minmax(0, 1fr)); }
        .metric, button.metric, .card.metric, .card {
          min-width:0; min-height:84px; display:flex; flex-direction:column; justify-content:space-between;
          padding:11px 12px; border:1px solid var(--line); border-radius:8px; background:var(--surface);
          box-shadow:var(--shadow); text-align:left; cursor:pointer; color:inherit;
        }
        button.metric:hover, .card:hover { border-color:#cbd5e1; }
        .metric.active, .card.active, .card.metric.active { border-color:var(--primary); box-shadow:inset 0 0 0 1px var(--primary); }
        .metric-label, .metric .k, .card .k {
          min-height:34px; display:flex; align-items:flex-start; color:var(--muted); font-size:12px; line-height:1.35;
          overflow-wrap:break-word; word-break:normal;
        }
        .metric-value, .metric .v, .card .v { font-size:24px; line-height:1; font-weight:760; color:#0f172a; }
        .metric.warning .metric-value, .metric.warning .v { color:var(--amber); }
        .autoban-summary .metric-value, .ban-summary .metric-value, .ban-summary .v { font-size:23px; }

        .results { margin-top:12px; overflow:hidden; min-width:0; }
        .results-head {
          display:flex; align-items:center; justify-content:space-between; gap:12px; padding:10px 12px;
          border-bottom:1px solid var(--line); min-width:0;
        }
        .filter-context, .hint, .pager-meta, .pager-meta-row { color:var(--muted); font-size:12px; min-width:0; }
        .progress-wrap { padding:10px 12px; border-bottom:1px solid var(--line); background:var(--surface-muted); min-width:0; }
        .progress, .progress-copy { margin:0; color:var(--muted); font-size:12px; min-width:0; }
        .progress.live, .progress-copy { color:#1d4ed8; font-weight:660; }
        .err { color:var(--red); white-space:pre-wrap; font-size:12px; }
        .table-wrap, .table-wrap.account-pool {
          background:transparent; border:0; border-radius:0; box-shadow:none; overflow:hidden; width:100%; min-width:0;
        }
        .table-scroll, .table-wrap.account-pool .table-scroll {
          overflow-x:auto; overflow-y:hidden; -webkit-overflow-scrolling:touch; width:100%; max-width:100%; min-width:0;
        }
        table { width:100%; border-collapse:collapse; table-layout:auto; font-size:13px; }
        .table-wrap.account-pool table,
        .table-wrap.account-pool table.inspect-table,
        .table-wrap.account-pool table.ban-table,
        .autoban-table {
          width:100%; min-width:1100px; table-layout:auto; border-collapse:collapse; font-size:13px;
        }
        th { padding:10px 12px; background:linear-gradient(180deg,#f8fafc 0%,#f1f5f9 100%); color:#475569; font-size:11px; text-align:left; white-space:nowrap; }
        td { padding:11px 12px; border-top:1px solid var(--line); vertical-align:middle; color:#334155; }
        /* Show full account ids; horizontal scroll instead of mid-email ellipsis. */
        .table-wrap.account-pool table.ban-table td.col-name,
        .table-wrap.account-pool table.ban-table td:first-child,
        .table-wrap.account-pool table.inspect-table td.col-name,
        .table-wrap.account-pool table.inspect-table td.account,
        td.col-name, td.account {
          color:#0f172a; white-space:normal; overflow:visible; text-overflow:clip;
          word-break:break-all; overflow-wrap:anywhere; max-width:none;
        }
        td.col-reason { white-space:normal; overflow-wrap:anywhere; word-break:break-word; }
        th.col-status, td.col-status, th.col-result, td.col-result { white-space:nowrap; width:1%; min-width:88px; }
        th.col-http, td.col-http { white-space:nowrap; width:1%; min-width:56px; text-align:center; }
        th.col-model, td.col-model { white-space:nowrap; min-width:72px; }
        th.col-action, td.col-action { white-space:nowrap; min-width:72px; }
        th.col-ops, td.col-ops { white-space:nowrap; width:1%; min-width:100px; }
        .badge, .pill {
          display:inline-flex; align-items:center; min-height:24px; padding:3px 8px; border-radius:6px;
          font-size:11px; font-weight:720; white-space:nowrap; flex-shrink:0;
        }
        .row-actions { display:flex; align-items:center; gap:5px; flex-wrap:wrap; }
        .row-actions .icon-btn, .row-actions button {
          width:30px; height:30px; min-height:30px; padding:0; border:1px solid var(--line); border-radius:6px; background:var(--surface);
        }
        .row-actions .danger-icon, .row-actions .danger { color:#b91c1c; border-color:#fecaca; background:#fef2f2; }
        tr.row-busy { opacity:.55; }
        .empty {
          min-height:140px; display:flex; align-items:center; justify-content:center; color:var(--muted);
          padding:48px 20px; text-align:center;
        }
        .pager, .table-footer {
          display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap;
          padding:10px 12px; border-top:1px solid var(--line); color:var(--muted); font-size:12px;
          background:transparent; min-width:0;
        }
        .modal {
          position:fixed; inset:0; z-index:10050; display:flex; align-items:center; justify-content:center;
          background:rgba(15,23,42,.45); padding:16px;
        }
        .modal.hidden { display:none; }
        .modal-card {
          width:min(440px,100%); background:var(--surface); border-radius:12px; border:1px solid var(--line);
          box-shadow:0 20px 40px rgba(15,23,42,.18); padding:18px 18px 14px; color:var(--text);
        }
        .modal-title { font-size:16px; font-weight:700; margin-bottom:10px; }
        .modal-msg { font-size:13px; line-height:1.6; color:var(--muted); white-space:pre-wrap; margin-bottom:16px; }
        .modal-actions { display:flex; justify-content:flex-end; gap:8px; }

        html[data-grok-theme="dark"] {
          color-scheme: dark;
          --bg:#111827; --page-bg:#111827; --surface:#182131; --surface-muted:#151d2b; --surface-subtle:#1d2737; --surface-2:#1d2737; --line:#334155; --line-subtle:#273449; --line-strong:#475569; --input-border:#475569;
          --text:#f8fafc; --muted:#a7b3c7; --muted-2:#94a3b8; --primary:#2563eb; --primary-hover:#1d4ed8;
          --primary-soft:#242c58; --green:#bbf7d0; --green-soft:#14532d; --amber:#b45309;
          --red:#fecaca; --red-soft:#3f1d1d; --purple:#c7d2fe; --purple-soft:#252b63; --shadow:none;
        }
        html[data-grok-theme="dark"] body,
        html[data-grok-theme="dark"] .grok-inspection-page { background:var(--bg) !important; color:var(--text) !important; }
        html[data-grok-theme="dark"] .eyebrow { color:#c7d2fe; background:#252b63; }
        html[data-grok-theme="dark"] .tab.active,
        html[data-grok-theme="dark"] .mode-tab.active {
          color:#fff !important; background:#2563eb !important; border-color:#2563eb !important; border-bottom-color:#2563eb !important;
        }
        html[data-grok-theme="dark"] .tab.active .tab-desc,
        html[data-grok-theme="dark"] .mode-tab.active .tab-desc,
        html[data-grok-theme="dark"] .tab.active small,
        html[data-grok-theme="dark"] .mode-tab.active small { color:#fff !important; }
        html[data-grok-theme="dark"] .sampling-row,
        html[data-grok-theme="dark"] .progress-wrap { background:var(--surface-muted); }
        html[data-grok-theme="dark"] #managementKey,
        html[data-grok-theme="dark"] .access-value { color:#f8fafc; background:var(--surface-2); border-color:var(--line-strong); }
        html[data-grok-theme="dark"] .field,
        html[data-grok-theme="dark"] .check-control,
        html[data-grok-theme="dark"] .section-label { color:#f8fafc; background:var(--surface); border-color:var(--line); }
        html[data-grok-theme="dark"] .btn:not(.primary):not(.soft):not(.danger),
        html[data-grok-theme="dark"] .grok-inspection-page button:not(.primary):not(.soft):not(.danger):not(.tab):not(.mode-tab):not(.icon-btn):not(.metric) {
          color:#f8fafc; background:var(--surface); border-color:var(--line);
        }
        html[data-grok-theme="dark"] .grok-inspection-page .btn.soft,
        html[data-grok-theme="dark"] .grok-inspection-page button.soft {
          color:#dbe4ff !important; background:#242c58 !important; border-color:#4b5aa6 !important;
        }
        html[data-grok-theme="dark"] .grok-inspection-page .btn.danger,
        html[data-grok-theme="dark"] .grok-inspection-page button.danger {
          color:#fecaca !important; background:#3f1d1d !important; border-color:#7f1d1d !important;
        }
        html[data-grok-theme="dark"] .grok-inspection-page .btn.primary,
        html[data-grok-theme="dark"] .grok-inspection-page button.primary {
          color:#fff !important; background:#2563eb !important; border-color:#2563eb !important;
        }
        html[data-grok-theme="dark"] .select-compact,
        html[data-grok-theme="dark"] .grok-inspection-page select,
        html[data-grok-theme="dark"] .pager select {
          color:#f8fafc; background:var(--surface); border-color:var(--line-strong); color-scheme:dark;
        }
        html[data-grok-theme="dark"] th { color:#a7b3c7; background:#1d2737; }
        html[data-grok-theme="dark"] td { color:#f8fafc; }
        html[data-grok-theme="dark"] td.col-name, html[data-grok-theme="dark"] .account { color:#f8fafc; }
        html[data-grok-theme="dark"] .metric-value,
        html[data-grok-theme="dark"] .card .v,
        html[data-grok-theme="dark"] .metric .v { color:#f8fafc; }
        html[data-grok-theme="dark"] .autoban-heading-icon {
          color:#c7d2fe; background:#252b63;
        }
        html[data-grok-theme="dark"] .slider, html[data-grok-theme="dark"] .toggle-track { background:#475569; }
        html[data-grok-theme="dark"] .switch input:checked + .slider,
        html[data-grok-theme="dark"] .toggle input:checked + .toggle-track { background:#16a34a; }
        html[data-grok-theme="dark"] .status-pill.on { background:#14532d; color:#bbf7d0; }
        html[data-grok-theme="dark"] .status-pill.off,
        html[data-grok-theme="dark"] .grok-inspection-page .status-pill.off { background:#7f1d1d; color:#fecaca; }
        html[data-grok-theme="dark"] .help-popover {
          background:var(--surface); border-color:var(--line); color:var(--muted); box-shadow:0 12px 32px rgba(0,0,0,.45);
        }
        html[data-grok-theme="dark"] .modal-card { background:var(--surface); border-color:var(--line); color:var(--text); }
        html[data-grok-theme="dark"] .modal-msg { color:var(--muted); }
        html[data-grok-theme="dark"] .row-actions .icon-btn, html[data-grok-theme="dark"] .icon-btn {
          color:#f8fafc; background:var(--surface); border-color:var(--line-strong);
        }
        html[data-grok-theme="dark"] .title-row .help-btn {
          background:transparent; border-color:transparent; color:#94a3b8;
        }
        html[data-grok-theme="dark"] .title-row .help-btn:hover,
        html[data-grok-theme="dark"] .title-row .help-btn[aria-expanded="true"] {
          background:#252b63; border-color:transparent; color:#c7d2fe;
        }
        html[data-grok-theme="dark"] .row-actions .danger-icon { color:#fecaca; background:#3f1d1d; border-color:#7f1d1d; }
        @media (prefers-color-scheme: dark) {
          html:not([data-grok-theme="light"]) {
            color-scheme: dark;
            --bg:#111827; --page-bg:#111827; --surface:#182131; --surface-muted:#151d2b; --surface-subtle:#1d2737; --surface-2:#1d2737; --line:#334155; --line-subtle:#273449; --line-strong:#475569; --input-border:#475569;
            --text:#f8fafc; --muted:#a7b3c7; --muted-2:#94a3b8; --primary:#2563eb; --primary-hover:#1d4ed8;
            --primary-soft:#242c58; --green:#bbf7d0; --green-soft:#14532d; --amber:#b45309;
            --red:#fecaca; --red-soft:#3f1d1d; --purple:#c7d2fe; --purple-soft:#252b63; --shadow:none;
          }
        }

        @media (max-width:1220px) {
          .summary, .overview.inspection-summary { grid-template-columns:repeat(4, minmax(0, 1fr)); }
          .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns:repeat(3, minmax(0, 1fr)); }
          .toolbar { grid-template-columns:1fr; }
          .toolbar-actions { justify-content:flex-start; }
        }
        @media (max-width:1180px) and (min-width:901px) {
          .schedule-row { gap:6px; }
          .schedule-controls { gap:4px; }
          .schedule-controls .field, .schedule-controls .check-control { padding:0 6px; gap:4px; }
          .schedule-controls .field label, .schedule-controls .check-control span { max-width:7.5em; }
          .schedule-controls .field.wide label { max-width:3.5em; }
          .schedule-status, #scheduleStatus { max-width:9em; }
        }
        @media (max-width:900px) {
          .grok-inspection-page { width:min(100% - 24px, 1480px); padding-top:12px; }
          .help-popover { top:40px; left:0; right:auto; }
          .summary, .overview.inspection-summary { grid-template-columns:repeat(3, minmax(0, 1fr)); }
          .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns:repeat(3, minmax(0, 1fr)); }
          .schedule-row { grid-template-columns:1fr auto; gap:8px; }
          .schedule-controls {
            grid-column:1 / -1; flex-wrap:wrap; overflow:visible; gap:6px;
          }
          .schedule-controls .field label, .schedule-controls .check-control span { max-width:none; }
          .schedule-status, #scheduleStatus { max-width:none; }
        }
        @media (max-width:760px) {
          body { overflow-x:hidden !important; }
          .grok-inspection-page { width:100%; max-width:100%; padding:10px 10px 28px; }
          .mode-tabs, .grok-inspection-page .tabs {
            display:grid !important; grid-template-columns:1fr 1fr !important; width:100%; max-width:100%; gap:4px; padding:8px;
          }
          .tab, .mode-tab, button.tab, button.mode-tab { width:100%; min-width:0; max-width:100%; padding:8px; align-items:center; }
          .tab > span, .mode-tab > span { min-width:0; overflow:hidden; }
          .tab .tab-title, .mode-tab .tab-title, .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small {
            overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
          }
          .summary, .overview, .summary.ban-summary, .autoban-summary {
            grid-template-columns:repeat(2,minmax(0,1fr)) !important;
          }
          .table-scroll, .table-wrap.account-pool .table-scroll { overflow-x:auto; max-width:100%; }
          .table-wrap.account-pool table,
          .table-wrap.account-pool table.inspect-table,
          .table-wrap.account-pool table.ban-table { min-width:920px !important; table-layout:auto !important; }
          .autoban-bar { flex-direction:column; align-items:flex-start; }
          .autoban-actions { flex-direction:column; align-items:flex-start; }
        }
        @media (max-width:640px) {
          body { font-size:13px; overflow-x:hidden !important; }
          .grok-inspection-page { width:100%; max-width:100%; padding:10px 10px 28px; }
          .page-head { position:relative; }
          .help-wrap { position:static; }
          h1 { font-size:20px; }
          .head-actions { padding-top:22px; }
          .lang-ctl .select-compact, .lang-ctl select { width:72px; min-width:72px; }
          .help-popover {
            top:calc(100% + 8px); left:0; right:0; width:auto;
            max-height:min(36vh, 200px); overflow-y:auto;
          }
          .panel, .autoban-control, #panel-autoban { border-radius:7px; }
          .access-row { flex-direction:column; align-items:flex-start; gap:6px; }
          #managementKey, .access-value { width:100%; min-width:0; }
          .mode-tabs, .grok-inspection-page .tabs {
            display:grid !important; grid-template-columns:1fr 1fr !important; width:100%; max-width:100%; gap:4px; padding:8px 8px 0;
          }
          .tab, .mode-tab { width:100%; min-width:0; max-width:100%; padding:8px; align-items:center; }
          .tab > span, .mode-tab > span { min-width:0; overflow:hidden; }
          .tab svg, .mode-tab svg { margin-top:0; }
          .tab .tab-title, .mode-tab .tab-title, .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small {
            overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
          }
          .toolbar { grid-template-columns:1fr; padding:10px; }
          .toolbar-main, .toolbar-actions { display:grid; grid-template-columns:repeat(2, minmax(0, 1fr)); }
          .toolbar .field, .toolbar .check-control, .toolbar .btn { width:100%; min-width:0; }
          .toolbar-actions .primary, .toolbar-actions #runBtn { grid-column:1 / -1; }
          .sampling-row { display:block; padding:10px; }
          .sampling-controls { display:grid; grid-template-columns:1fr 1fr; margin-top:8px; }
          .sampling-controls .field { width:100%; min-width:0; }
          .sampling-controls .btn, .sampling-controls #sampleBtn { grid-column:1 / -1; width:100%; }
          .schedule-row { display:grid; grid-template-columns:1fr auto; padding:10px; }
          .schedule-controls { grid-column:1 / -1; display:grid; grid-template-columns:1fr 1fr; flex-wrap:wrap; overflow:visible; }
          .schedule-controls .field, .schedule-controls .check-control { min-width:0; width:100%; }
          .schedule-controls .wide { grid-column:1 / -1; }
          .summary, .overview.inspection-summary, .summary.ban-summary, .overview.autoban-summary, .autoban-summary {
            grid-template-columns:repeat(2, minmax(0, 1fr)) !important; gap:7px;
          }
          .metric, .card { min-height:82px; padding:10px; }
          .metric-label, .card .k { min-height:33px; font-size:11px; }
          .metric-value, .card .v { font-size:22px; }
          .results-head { align-items:flex-start; flex-direction:column; }
          .bulk-actions { display:grid; grid-template-columns:1fr 1fr; width:100%; }
          .bulk-actions .btn, .bulk-actions button { width:100%; min-width:0; }
          .result-tools { width:100%; justify-content:space-between; }
          .table-footer, .pager { align-items:flex-start; flex-direction:column; }
          .pager > div { width:100%; }
          .autoban-bar { align-items:flex-start; flex-direction:column; gap:10px; }
          .autoban-switch-row { width:100%; justify-content:space-between; }
          .autoban-actions { align-items:flex-start; flex-direction:column; }
          .autoban-action-buttons { display:grid; grid-template-columns:1fr 1fr; width:100%; }
          .autoban-action-buttons .btn, .autoban-action-buttons button { width:100%; min-width:0; }
          .autoban-action-buttons .danger, .autoban-action-buttons #banUnbanAllBtn { grid-column:1 / -1; }
          .table-wrap.account-pool { width:100% !important; min-width:0 !important; border-radius:7px; }
          .table-scroll { overflow-x:auto; max-width:100%; }
          .table-wrap.account-pool table,
          .table-wrap.account-pool table.inspect-table,
          .table-wrap.account-pool table.ban-table { min-width:920px !important; table-layout:auto !important; }
        }

    .summary.ban-summary { grid-template-columns:repeat(6,minmax(0,1fr)); width:100%; min-width:0; }

    @media (min-width:761px) and (max-width:1220px) {
      .summary, .overview.inspection-summary { grid-template-columns:repeat(4,minmax(0,1fr)); }
      .summary.ban-summary, .autoban-summary { grid-template-columns:repeat(3,minmax(0,1fr)); }
    }
    html[data-grok-theme="dark"] .grok-inspection-page .tab.active { color:#fff !important; background:#2563eb !important; }
    html[data-grok-theme="dark"] .grok-inspection-page th { color:#a7b3c7; background:#1d2737; }

    /* Keep the released v0.1.16 component palette after the layout overrides. */
    html, body { background:var(--page-bg) !important; color:var(--text) !important; }
    .grok-inspection-page .title-row .help-btn {
      width:28px !important; height:28px !important; min-height:28px !important;
      padding:0 !important; border:1px solid transparent !important; border-radius:7px !important;
      background:transparent !important; color:var(--muted-2) !important;
    }
    .grok-inspection-page .title-row .help-btn:hover,
    .grok-inspection-page .title-row .help-btn[aria-expanded="true"] {
      background:var(--primary-soft) !important; border-color:transparent !important; color:var(--primary) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .title-row .help-btn {
      background:transparent !important; border-color:transparent !important; color:#94a3b8 !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .title-row .help-btn:hover,
    html[data-grok-theme="dark"] .grok-inspection-page .title-row .help-btn[aria-expanded="true"] {
      background:#252b63 !important; border-color:transparent !important; color:#c7d2fe !important;
    }
    .grok-inspection-page .progress { color:var(--muted) !important; }
    .grok-inspection-page .progress.live {
      color:#1d4ed8 !important; background:#dbeafe !important; border:1px solid transparent !important; border-color:#93c5fd !important;
      box-shadow:0 0 0 1px rgba(37,99,235,.08) !important;
    }
    .grok-inspection-page th {
      background:var(--surface-subtle) !important; color:var(--muted) !important; border-color:var(--line) !important;
    }
    .grok-inspection-page td { border-color:var(--line-subtle) !important; }
    .grok-inspection-page .pager,
    .grok-inspection-page .table-footer {
      background:var(--surface-muted) !important; border-color:var(--line) !important; color:var(--muted) !important;
    }
    .grok-inspection-page .access-value,
    .grok-inspection-page input[type="number"] {
      color:var(--text) !important; border-color:var(--input-border) !important; color-scheme:inherit;
    }
    html[data-grok-theme] .grok-inspection-page .field input[type="number"] {
      min-height:0 !important; height:auto !important; padding:0 !important; text-align:center !important;
      border:0 !important; border-radius:0 !important;
      background:transparent !important; box-shadow:none !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page table,
    html[data-grok-theme="dark"] .grok-inspection-page thead,
    html[data-grok-theme="dark"] .grok-inspection-page tbody,
    html[data-grok-theme="dark"] .grok-inspection-page tr,
    html[data-grok-theme="dark"] .grok-inspection-page td {
      background:transparent !important; color:var(--text) !important; border-color:var(--line-subtle) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page th {
      background:var(--surface-subtle) !important; color:var(--muted) !important; border-color:var(--line) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .pager,
    html[data-grok-theme="dark"] .grok-inspection-page .table-footer {
      background:var(--surface-muted) !important; border-color:var(--line) !important; color:var(--muted) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .progress.live {
      color:#93c5fd !important; background:#1e3a5f !important; border-color:#3b82f6 !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .progress.live::before { background:#60a5fa; }
    html[data-grok-theme="dark"] .grok-inspection-page .access-value,
    html[data-grok-theme="dark"] .grok-inspection-page input[type="number"] {
      background:var(--surface-subtle) !important; color:var(--text) !important; border-color:var(--input-border) !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .field input,
    html[data-grok-theme="dark"] .grok-inspection-page .field select {
      background:transparent !important; color:var(--text) !important; border-color:transparent !important;
    }
    html[data-grok-theme="dark"] .grok-inspection-page .badge { background:#252b63 !important; color:#c7d2fe !important; }
    html[data-grok-theme="dark"] .grok-inspection-page .metric.active,
    html[data-grok-theme="dark"] .grok-inspection-page .card.active { border-color:#60a5fa !important; }
    html[data-grok-theme="dark"] .modal-card { background:#1e293b !important; border-color:#334155 !important; color:#e5e7eb !important; }
    html[data-grok-theme="dark"] .modal-title { color:#f8fafc !important; }
    html[data-grok-theme="dark"] .modal-msg { color:#cbd5e1 !important; }
    @media (prefers-color-scheme: dark) {
      html:not([data-grok-theme="light"]) {
        --page-bg:#111827; --surface-subtle:#1d2737; --line-subtle:#273449; --input-border:#475569;
      }
      html:not([data-grok-theme="light"]) .autoban-heading-icon {
        color:#c7d2fe; background:#252b63;
      }
      html:not([data-grok-theme="light"]) .grok-inspection-page .progress.live {
        color:#93c5fd !important; background:#1e3a5f !important; border-color:#3b82f6 !important;
      }
      html:not([data-grok-theme="light"]) .grok-inspection-page .progress.live::before { background:#60a5fa; }
    }
`
