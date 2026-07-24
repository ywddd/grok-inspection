package main

const uiCSS = `
    :root {
          color-scheme: light;
          --bg: #f5f7fa;
          --surface: #ffffff;
          --surface-2: #f8fafc;
          --line: #dfe5ec;
          --line-strong: #cdd6e1;
          --text: #162235;
          --muted: #65748b;
          --muted-2: #8a98aa;
          --primary: #2563eb;
          --primary-hover: #1d4ed8;
          --primary-soft: #eef4ff;
          --green: #0f8a66;
          --green-soft: #e8f6f1;
          --amber: #a86108;
          --amber-soft: #fff5dd;
          --red: #c73838;
          --red-soft: #fff0f0;
          --purple: #7253c7;
          --purple-soft: #f2edff;
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
          border-radius:6px; color:#3d4eb0; background:#eef0ff; font-size:11px; font-weight:700;
        }
        .title-row { display:flex; align-items:center; gap:7px; min-width:0; }
        h1 { margin:0; font-size:23px; line-height:1.25; letter-spacing:0; }
        .help-wrap { position:relative; flex:0 0 auto; }
        .icon-btn, .help-btn {
          width:34px; height:34px; display:inline-grid; place-items:center; border:1px solid transparent;
          border-radius:7px; background:transparent; color:#596b82; padding:0;
        }
        .icon-btn:hover, .help-btn:hover { background:var(--surface-2); border-color:var(--line); color:var(--text); }
        .help-popover {
          position:absolute; z-index:50; top:40px; right:0; width:min(380px, calc(100vw - 32px));
          padding:12px 14px; border:1px solid var(--line-strong); border-radius:8px; background:var(--surface);
          box-shadow:0 12px 32px rgba(15,23,42,.14); color:var(--muted); font-size:13px;
          opacity:0; visibility:hidden; transform:translateY(-4px); transition:.15s ease;
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
          font-family:ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; color:#3e5068;
        }
        .key-state, #keyHint { display:inline-flex; align-items:center; gap:6px; color:var(--muted); font-weight:650; min-width:0; }
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
        .tab > span, .mode-tab > span { min-width:0; flex:1 1 auto; overflow:hidden; }
        .tab .tab-title, .mode-tab .tab-title {
          display:block; line-height:1.2; overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
        }
        .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small {
          display:block; margin-top:1px; color:var(--muted-2); font-size:11px; font-weight:520;
          overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
        }
        .tab.active, .mode-tab.active {
          color:#174aa7; background:var(--primary-soft); border-color:#d7e4fb; border-bottom-color:var(--primary-soft);
        }
        .tab.active .tab-desc, .mode-tab.active .tab-desc, .tab.active small, .mode-tab.active small { color:#5474a8; }
        .tab:focus-visible, .mode-tab:focus-visible { outline:2px solid var(--primary); outline-offset:-2px; }

        .toolbar {
          display:grid; grid-template-columns:minmax(0,1fr) auto; gap:10px; padding:12px;
          border-top:1px solid var(--line); min-width:0;
        }
        .toolbar-main, .toolbar-actions, .sampling-controls, .schedule-controls, .bulk-actions, .result-tools, .autoban-action-buttons {
          display:flex; align-items:center; flex-wrap:wrap; gap:8px; min-width:0;
        }
        .field, .check-control {
          min-height:34px; display:inline-flex; align-items:center; gap:8px; padding:0 10px;
          border:1px solid var(--line); border-radius:7px; background:var(--surface); color:#3b4a60; min-width:0;
        }
        .field label { color:var(--muted); font-size:12px; white-space:nowrap; }
        .field input {
          width:42px; min-width:0; padding:0; border:0; outline:0; background:transparent; text-align:right; font-weight:650;
        }
        .field select { border:0; background:transparent; outline:0; min-width:0; height:28px; padding:0 4px; }
        .check-control input { width:14px; height:14px; margin:0; accent-color:var(--primary); }
        .check-control span { white-space:nowrap; }
        .btn, .grok-inspection-page button.btn {
          min-height:34px; display:inline-flex; align-items:center; justify-content:center; gap:7px; padding:0 12px;
          border:1px solid var(--line-strong); border-radius:7px; background:var(--surface); color:#35445a;
          font-weight:650; white-space:nowrap;
        }
        .btn:hover { border-color:#aebaca; background:var(--surface-2); }
        .btn.primary, button.primary { border-color:var(--primary); color:#fff; background:var(--primary); }
        .btn.primary:hover, button.primary:hover { background:var(--primary-hover); }
        .btn.soft, button.soft { border-color:#cbdaf9; color:#315aa6; background:var(--primary-soft); }
        .btn.danger, button.danger { border-color:#f2caca; color:var(--red); background:var(--red-soft); }
        .btn[disabled], button:disabled { opacity:.5; cursor:default; }
        .sampling-row {
          display:flex; align-items:center; justify-content:space-between; gap:12px; padding:10px 12px;
          border-top:1px solid var(--line); background:#fbfcfe; min-width:0;
        }
        .section-label {
          display:inline-flex; align-items:center; gap:7px; color:#42536a; font-size:12px; font-weight:720; white-space:nowrap;
        }
        .schedule-row {
          display:grid; grid-template-columns:auto minmax(0,1fr) auto; align-items:center; gap:12px;
          padding:10px 12px; border-top:1px solid var(--line); min-width:0;
        }
        .schedule-status, #scheduleStatus {
          display:inline-flex; align-items:center; gap:6px; color:var(--muted); font-size:12px; font-weight:650; min-width:0;
        }
        .status-dot { width:7px; height:7px; border-radius:50%; background:#a9b3c2; flex:0 0 auto; }

        .autoban-control { margin-top:12px; overflow:hidden; }
        .autoban-bar {
          min-height:58px; display:flex; align-items:center; justify-content:space-between; gap:16px;
          padding:11px 12px; border-bottom:1px solid var(--line); min-width:0;
        }
        .autoban-heading { display:flex; align-items:center; gap:9px; min-width:0; }
        .autoban-heading-icon {
          width:34px; height:34px; display:grid; place-items:center; flex:0 0 auto; border-radius:7px;
          color:#315aa6; background:var(--primary-soft);
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
          position:absolute; inset:0; border-radius:12px; background:#b8c2cf; transition:.16s ease; cursor:pointer;
        }
        .slider:before, .toggle-track::after {
          content:''; position:absolute; top:3px; left:3px; width:18px; height:18px; border-radius:50%;
          background:#fff; box-shadow:0 1px 3px rgba(15,23,42,.28); transition:.16s ease;
        }
        .switch input:checked + .slider, .toggle input:checked + .toggle-track { background:var(--green); }
        .switch input:checked + .slider:before, .toggle input:checked + .toggle-track::after { transform:translateX(18px); }
        .status-pill {
          min-height:25px; display:inline-flex; align-items:center; padding:3px 8px; border-radius:6px;
          color:var(--green); background:var(--green-soft); font-size:11px; font-weight:730; white-space:nowrap;
        }
        .status-pill.off { color:var(--muted); background:var(--surface-2); }
        .status-pill.on { color:var(--green); background:var(--green-soft); }
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
        button.metric:hover, .card:hover { border-color:#b9c6d5; }
        .metric.active, .card.active, .card.metric.active { border-color:var(--primary); box-shadow:inset 0 0 0 1px var(--primary); }
        .metric-label, .metric .k, .card .k {
          min-height:34px; display:flex; align-items:flex-start; color:var(--muted); font-size:12px; line-height:1.35;
          overflow-wrap:break-word; word-break:normal;
        }
        .metric-value, .metric .v, .card .v { font-size:24px; line-height:1; font-weight:760; color:#142036; }
        .metric.warning .metric-value, .metric.warning .v { color:var(--amber); }
        .autoban-summary .metric-value, .ban-summary .metric-value, .ban-summary .v { font-size:23px; }

        .results { margin-top:12px; overflow:hidden; min-width:0; }
        .results-head {
          display:flex; align-items:center; justify-content:space-between; gap:12px; padding:10px 12px;
          border-bottom:1px solid var(--line); min-width:0;
        }
        .filter-context, .hint, .pager-meta, .pager-meta-row { color:var(--muted); font-size:12px; min-width:0; }
        .progress-wrap { padding:10px 12px; border-bottom:1px solid var(--line); background:#fbfcfe; min-width:0; }
        .progress, .progress-copy { margin:0; color:var(--muted); font-size:12px; min-width:0; }
        .progress.live, .progress-copy { color:#31568d; font-weight:660; }
        .err { color:var(--red); white-space:pre-wrap; font-size:12px; }
        .table-wrap, .table-wrap.account-pool {
          background:transparent; border:0; border-radius:0; box-shadow:none; overflow:hidden; width:100%; min-width:0;
        }
        .table-scroll, .table-wrap.account-pool .table-scroll {
          overflow-x:auto; overflow-y:hidden; -webkit-overflow-scrolling:touch; width:100%; max-width:100%; min-width:0;
        }
        table { width:100%; border-collapse:collapse; table-layout:fixed; font-size:13px; }
        .table-wrap.account-pool table.inspect-table { min-width:970px; }
        .table-wrap.account-pool table.ban-table, .autoban-table { min-width:1080px; }
        th { padding:10px 12px; background:#f5f8fb; color:#5a6b82; font-size:11px; text-align:left; white-space:nowrap; }
        td { padding:11px 12px; border-top:1px solid var(--line); vertical-align:middle; color:#425269; }
        td.col-name, td.account { color:#324965; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
        td.col-reason { white-space:normal; overflow-wrap:anywhere; word-break:break-word; }
        .badge, .pill {
          display:inline-flex; align-items:center; min-height:24px; padding:3px 8px; border-radius:6px;
          font-size:11px; font-weight:720; white-space:nowrap; flex-shrink:0;
        }
        .row-actions { display:flex; align-items:center; gap:5px; flex-wrap:wrap; }
        .row-actions .icon-btn, .row-actions button {
          width:30px; height:30px; min-height:30px; padding:0; border:1px solid var(--line); border-radius:6px; background:var(--surface);
        }
        .row-actions .danger-icon, .row-actions .danger { color:var(--red); border-color:#f2d0d0; background:#fff8f8; }
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
          --bg:#0f151d; --surface:#171f2a; --surface-2:#111923; --line:#2a3543; --line-strong:#3b4858;
          --text:#edf2f8; --muted:#a6b1c1; --muted-2:#8190a4; --primary:#4b83ee; --primary-hover:#5b91f7;
          --primary-soft:#1a2c4c; --green:#5bd5ad; --green-soft:#17362f; --amber:#efba62; --amber-soft:#3a2b15;
          --red:#f27f7f; --red-soft:#3c2023; --purple:#b7a1ff; --purple-soft:#2d2448; --shadow:none;
        }
        html[data-grok-theme="dark"] body,
        html[data-grok-theme="dark"] .grok-inspection-page { background:var(--bg) !important; color:var(--text) !important; }
        html[data-grok-theme="dark"] .eyebrow { color:#b9c4ff; background:#29315a; }
        html[data-grok-theme="dark"] .tab.active,
        html[data-grok-theme="dark"] .mode-tab.active {
          color:#c9ddff; background:var(--primary-soft); border-color:#30476a; border-bottom-color:var(--primary-soft);
        }
        html[data-grok-theme="dark"] .tab.active .tab-desc,
        html[data-grok-theme="dark"] .mode-tab.active .tab-desc,
        html[data-grok-theme="dark"] .tab.active small,
        html[data-grok-theme="dark"] .mode-tab.active small { color:#9eb6dd; }
        html[data-grok-theme="dark"] .sampling-row,
        html[data-grok-theme="dark"] .progress-wrap { background:#131b25; }
        html[data-grok-theme="dark"] #managementKey,
        html[data-grok-theme="dark"] .access-value { color:#b8c5d6; background:var(--surface-2); border-color:var(--line); }
        html[data-grok-theme="dark"] .field,
        html[data-grok-theme="dark"] .check-control,
        html[data-grok-theme="dark"] .section-label { color:#c9d4e2; background:var(--surface); border-color:var(--line); }
        html[data-grok-theme="dark"] .btn,
        html[data-grok-theme="dark"] .grok-inspection-page button:not(.primary):not(.soft):not(.danger):not(.tab):not(.mode-tab):not(.icon-btn):not(.metric) {
          color:#d8e2ee; background:var(--surface); border-color:var(--line-strong);
        }
        html[data-grok-theme="dark"] .btn.soft, html[data-grok-theme="dark"] button.soft {
          color:#bfd5ff; background:var(--primary-soft); border-color:#35517c;
        }
        html[data-grok-theme="dark"] .btn.danger, html[data-grok-theme="dark"] button.danger {
          color:#ffaaaa; background:var(--red-soft); border-color:#67383d;
        }
        html[data-grok-theme="dark"] .btn.primary, html[data-grok-theme="dark"] button.primary {
          color:#fff; background:var(--primary); border-color:var(--primary);
        }
        html[data-grok-theme="dark"] .select-compact,
        html[data-grok-theme="dark"] .grok-inspection-page select,
        html[data-grok-theme="dark"] .pager select {
          color:#dce5f0; background:var(--surface); border-color:var(--line-strong); color-scheme:dark;
        }
        html[data-grok-theme="dark"] th { color:#a9b7c9; background:#111923; }
        html[data-grok-theme="dark"] td { color:#bdc9d8; }
        html[data-grok-theme="dark"] td.col-name, html[data-grok-theme="dark"] .account { color:#dbe5f2; }
        html[data-grok-theme="dark"] .metric-value,
        html[data-grok-theme="dark"] .card .v,
        html[data-grok-theme="dark"] .metric .v { color:#f4f7fb; }
        html[data-grok-theme="dark"] .autoban-heading-icon { color:#bcd4ff; }
        html[data-grok-theme="dark"] .slider, html[data-grok-theme="dark"] .toggle-track { background:#485568; }
        html[data-grok-theme="dark"] .status-pill.on { color:var(--green); background:var(--green-soft); }
        html[data-grok-theme="dark"] .status-pill.off,
        html[data-grok-theme="dark"] .grok-inspection-page .status-pill { color:var(--muted); background:var(--surface-2); }
        html[data-grok-theme="dark"] .help-popover {
          background:var(--surface); border-color:var(--line); color:var(--muted); box-shadow:0 12px 32px rgba(0,0,0,.45);
        }
        html[data-grok-theme="dark"] .modal-card { background:var(--surface); border-color:var(--line); color:var(--text); }
        html[data-grok-theme="dark"] .modal-msg { color:var(--muted); }
        html[data-grok-theme="dark"] .row-actions .icon-btn, html[data-grok-theme="dark"] .icon-btn {
          color:#c7d2df; background:var(--surface); border-color:var(--line-strong);
        }
        html[data-grok-theme="dark"] .row-actions .danger-icon { color:#ff9b9b; background:var(--red-soft); border-color:#67383d; }
        @media (prefers-color-scheme: dark) {
          html:not([data-grok-theme="light"]) {
            color-scheme: dark;
            --bg:#0f151d; --surface:#171f2a; --surface-2:#111923; --line:#2a3543; --line-strong:#3b4858;
            --text:#edf2f8; --muted:#a6b1c1; --muted-2:#8190a4; --primary:#4b83ee; --primary-hover:#5b91f7;
            --primary-soft:#1a2c4c; --green:#5bd5ad; --green-soft:#17362f; --amber:#efba62; --amber-soft:#3a2b15;
            --red:#f27f7f; --red-soft:#3c2023; --purple:#b7a1ff; --purple-soft:#2d2448; --shadow:none;
          }
        }

        @media (max-width:1220px) {
          .summary, .overview.inspection-summary { grid-template-columns:repeat(4, minmax(0, 1fr)); }
          .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns:repeat(3, minmax(0, 1fr)); }
          .toolbar { grid-template-columns:1fr; }
          .toolbar-actions { justify-content:flex-start; }
        }
        @media (max-width:900px) {
          .grok-inspection-page { width:min(100% - 24px, 1480px); padding-top:12px; }
          .summary, .overview.inspection-summary { grid-template-columns:repeat(3, minmax(0, 1fr)); }
          .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns:repeat(3, minmax(0, 1fr)); }
          .schedule-row { grid-template-columns:1fr auto; }
          .schedule-controls { grid-column:1 / -1; }
        }
        @media (max-width:760px) {
          body { overflow-x:hidden !important; }
          .grok-inspection-page { width:100%; max-width:100%; padding:10px 10px 28px; }
          .mode-tabs, .grok-inspection-page .tabs {
            display:grid !important; grid-template-columns:1fr 1fr !important; width:100%; max-width:100%; gap:6px; padding:8px;
          }
          .tab, .mode-tab, button.tab, button.mode-tab { width:100%; min-width:0; max-width:100%; padding:8px; align-items:flex-start; }
          .tab > span, .mode-tab > span { min-width:0; overflow:hidden; }
          .tab .tab-title, .mode-tab .tab-title, .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small {
            overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
          }
          .summary, .overview, .summary.ban-summary, .autoban-summary {
            grid-template-columns:repeat(2,minmax(0,1fr)) !important;
          }
          .table-scroll, .table-wrap.account-pool .table-scroll { overflow-x:auto; max-width:100%; }
          .table-wrap.account-pool table.inspect-table, .table-wrap.account-pool table.ban-table { min-width:720px !important; }
          .autoban-bar { flex-direction:column; align-items:flex-start; }
          .autoban-actions { flex-direction:column; align-items:flex-start; }
        }
        @media (max-width:640px) {
          body { font-size:13px; overflow-x:hidden !important; }
          .grok-inspection-page { width:100%; max-width:100%; padding:10px 10px 28px; }
          .page-head { gap:10px; }
          h1 { font-size:20px; }
          .head-actions { padding-top:22px; }
          .lang-ctl .select-compact, .lang-ctl select { width:72px; min-width:72px; }
          .help-popover { left:0; right:auto; width:min(360px, calc(100vw - 24px)); }
          .panel, .autoban-control, #panel-autoban { border-radius:7px; }
          .access-row { flex-direction:column; align-items:flex-start; gap:6px; }
          #managementKey, .access-value { width:100%; min-width:0; }
          .mode-tabs, .grok-inspection-page .tabs {
            display:grid !important; grid-template-columns:1fr 1fr !important; width:100%; max-width:100%; padding:8px 8px 0;
          }
          .tab, .mode-tab { width:100%; min-width:0; max-width:100%; padding:8px; align-items:flex-start; }
          .tab > span, .mode-tab > span { min-width:0; overflow:hidden; }
          .tab svg, .mode-tab svg { margin-top:2px; }
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
          .schedule-controls { grid-column:1 / -1; display:grid; grid-template-columns:1fr 1fr; }
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
          .table-wrap.account-pool table.inspect-table, .table-wrap.account-pool table.ban-table { min-width:720px !important; }
        }

    .summary.ban-summary { grid-template-columns:repeat(6,minmax(0,1fr)); width:100%; min-width:0; }

    @media (min-width:761px) and (max-width:1220px) {
      .summary, .overview.inspection-summary { grid-template-columns:repeat(4,minmax(0,1fr)); }
      .summary.ban-summary, .autoban-summary { grid-template-columns:repeat(3,minmax(0,1fr)); }
    }
    html[data-grok-theme="dark"] .grok-inspection-page .tab.active { color:#c9ddff; background:var(--primary-soft); }
    html[data-grok-theme="dark"] .grok-inspection-page th { color:#a9b7c9; background:#111923; }
`
