package main

const uiCSS = `    :root {
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
          --sidebar: 220px;
          --topbar: 52px;
        }

        * { box-sizing: border-box; }
        [hidden] { display: none !important; }

        button, input, select { font: inherit; color: inherit; }
        button { cursor: pointer; }
        svg { width: 16px; height: 16px; stroke-width: 1.8; }



        .icon-btn:hover { background: var(--surface-2); border-color: var(--line); color: var(--text); }

        .page-head {
          display: flex;
          align-items: flex-start;
          justify-content: space-between;
          gap: 18px;
          margin-bottom: 14px;
        }

        .eyebrow {
          display: inline-flex;
          align-items: center;
          min-height: 24px;
          margin-bottom: 5px;
          padding: 3px 8px;
          border-radius: 6px;
          color: #3d4eb0;
          background: #eef0ff;
          font-size: 11px;
          font-weight: 700;
        }

        .title-row { display: flex; align-items: center; gap: 7px; }
        h1 { margin: 0; font-size: 23px; line-height: 1.25; letter-spacing: 0; }
        .help-wrap { position: relative; }

        .help-popover {
          position: absolute;
          z-index: 50;
          top: 40px;
          right: 0;
          width: min(380px, calc(100vw - 32px));
          padding: 12px 14px;
          border: 1px solid var(--line-strong);
          border-radius: 8px;
          background: var(--surface);
          box-shadow: 0 12px 32px rgba(15, 23, 42, .14);
          color: var(--muted);
          font-size: 13px;
          opacity: 0;
          visibility: hidden;
          transform: translateY(-4px);
          transition: .15s ease;
        }

        .help-popover.open { opacity: 1; visibility: visible; transform: translateY(0); }
        .help-popover strong { color: var(--text); }
        .help-popover p { margin: 0; }
        .help-popover p + p { margin-top: 8px; }

        .head-actions { display: flex; align-items: center; gap: 8px; padding-top: 25px; }

        .select-compact {
          height: 34px;
          padding: 0 30px 0 10px;
          border: 1px solid var(--line-strong);
          border-radius: 7px;
          background: var(--surface);
        }

        .panel {
          border: 1px solid var(--line);
          border-radius: 8px;
          background: var(--surface);
          box-shadow: var(--shadow);
        }

        .access-row {
          min-height: 48px;
          display: flex;
          align-items: center;
          gap: 12px;
          padding: 8px 12px;
          border-bottom: 1px solid var(--line);
        }

        .access-value {
          min-width: 225px;
          height: 32px;
          display: flex;
          align-items: center;
          padding: 0 10px;
          border: 1px solid var(--line);
          border-radius: 7px;
          background: var(--surface-2);
          font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
          color: #3e5068;
        }

        .key-state { display: inline-flex; align-items: center; gap: 6px; color: var(--green); font-weight: 650; }
        .key-state svg { width: 15px; height: 15px; }

        .mode-tabs {
          display: flex;
          align-items: stretch;
          padding: 10px 12px 0;
          gap: 4px;
        }

        .tab, .mode-tab {
          min-height: 48px;
          display: flex;
          align-items: center;
          gap: 9px;
          padding: 8px 14px;
          border: 1px solid transparent;
          border-radius: 7px 7px 0 0;
          background: transparent;
          color: var(--muted);
          font-weight: 680;
        }

        .tab small, .mode-tab small { display: block; margin-top: 1px; color: var(--muted-2); font-size: 11px; font-weight: 520; }
        .tab.active, .mode-tab.active { color: #174aa7; background: var(--primary-soft); border-color: #d7e4fb; border-bottom-color: var(--primary-soft); }
        .tab.active small, .mode-tab.active small { color: #5474a8; }
        .tab:focus-visible, .mode-tab:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }

        .toolbar {
          display: grid;
          grid-template-columns: minmax(0, 1fr) auto;
          gap: 10px;
          padding: 12px;
          border-top: 1px solid var(--line);
        }

        .toolbar-main, .toolbar-actions, .sampling-controls, .schedule-controls {
          display: flex;
          align-items: center;
          flex-wrap: wrap;
          gap: 8px;
        }

        .field, .check-control {
          min-height: 34px;
          display: inline-flex;
          align-items: center;
          gap: 8px;
          padding: 0 10px;
          border: 1px solid var(--line);
          border-radius: 7px;
          background: var(--surface);
          color: #3b4a60;
        }

        .field label { color: var(--muted); font-size: 12px; white-space: nowrap; }
        .field input {
          width: 42px;
          min-width: 0;
          padding: 0;
          border: 0;
          outline: 0;
          background: transparent;
          text-align: right;
          font-weight: 650;
        }

        .check-control input { width: 14px; height: 14px; margin: 0; accent-color: var(--primary); }
        .check-control span { white-space: nowrap; }

        .btn {
          min-height: 34px;
          display: inline-flex;
          align-items: center;
          justify-content: center;
          gap: 7px;
          padding: 0 12px;
          border: 1px solid var(--line-strong);
          border-radius: 7px;
          background: var(--surface);
          color: #35445a;
          font-weight: 650;
          white-space: nowrap;
        }

        .btn:hover { border-color: #aebaca; background: var(--surface-2); }
        .btn.primary { border-color: var(--primary); color: #fff; background: var(--primary); }
        .btn.primary:hover { background: var(--primary-hover); }
        .btn.soft { border-color: #cbdaf9; color: #315aa6; background: var(--primary-soft); }
        .btn.danger { border-color: #f2caca; color: var(--red); background: var(--red-soft); }
        .btn[disabled] { opacity: .5; cursor: default; }

        .sampling-row {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 12px;
          padding: 10px 12px;
          border-top: 1px solid var(--line);
          background: #fbfcfe;
        }

        .section-label {
          display: inline-flex;
          align-items: center;
          gap: 7px;
          color: #42536a;
          font-size: 12px;
          font-weight: 720;
          white-space: nowrap;
        }

        .sampling-controls { flex: 1; }
        .sample-summary { color: var(--muted); font-size: 12px; white-space: nowrap; }

        .schedule-row {
          display: grid;
          grid-template-columns: auto minmax(0, 1fr) auto;
          align-items: center;
          gap: 12px;
          padding: 10px 12px;
          border-top: 1px solid var(--line);
        }

        .schedule-status {
          display: inline-flex;
          align-items: center;
          gap: 6px;
          color: var(--muted);
          font-size: 12px;
          font-weight: 650;
          white-space: nowrap;
        }

        .status-dot { width: 7px; height: 7px; border-radius: 50%; background: #a9b3c2; }

        .autoban-control {
          margin-top: 12px;
          overflow: hidden;
        }

        .autoban-bar {
          min-height: 58px;
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 16px;
          padding: 11px 12px;
          border-bottom: 1px solid var(--line);
        }

        .autoban-heading {
          display: flex;
          align-items: center;
          gap: 9px;
          min-width: 0;
        }

        .autoban-heading-icon {
          width: 34px;
          height: 34px;
          display: grid;
          place-items: center;
          flex: 0 0 auto;
          border-radius: 7px;
          color: #315aa6;
          background: var(--primary-soft);
        }

        .autoban-heading strong { display: block; font-size: 15px; line-height: 1.25; }
        .autoban-heading small { display: block; margin-top: 2px; color: var(--muted); font-size: 11px; }

        .autoban-switch-row { display: flex; align-items: center; gap: 9px; }

        .toggle {
          position: relative;
          width: 42px;
          height: 24px;
          flex: 0 0 auto;
        }

        .toggle input {
          position: absolute;
          width: 1px;
          height: 1px;
          opacity: 0;
          pointer-events: none;
        }

        .toggle-track {
          position: absolute;
          inset: 0;
          border-radius: 12px;
          background: #b8c2cf;
          transition: .16s ease;
        }

        .toggle-track::after {
          content: '';
          position: absolute;
          top: 3px;
          left: 3px;
          width: 18px;
          height: 18px;
          border-radius: 50%;
          background: #fff;
          box-shadow: 0 1px 3px rgba(15, 23, 42, .28);
          transition: .16s ease;
        }

        .toggle input:checked + .toggle-track { background: var(--green); }
        .toggle input:checked + .toggle-track::after { transform: translateX(18px); }
        .toggle input:focus-visible + .toggle-track { outline: 2px solid var(--primary); outline-offset: 2px; }

        .status-pill {
          min-height: 25px;
          display: inline-flex;
          align-items: center;
          padding: 3px 8px;
          border-radius: 6px;
          color: var(--green);
          background: var(--green-soft);
          font-size: 11px;
          font-weight: 730;
          white-space: nowrap;
        }

        .status-pill.off { color: var(--muted); background: var(--surface-2); }

        .autoban-actions {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 12px;
          padding: 10px 12px;
        }

        .autoban-action-buttons { display: flex; align-items: center; flex-wrap: wrap; gap: 7px; }
        .autoban-pool-status { color: var(--muted); font-size: 12px; white-space: nowrap; }

        .overview {
          display: grid;
          grid-template-columns: repeat(7, minmax(132px, 1fr));
          gap: 8px;
          margin-top: 12px;
        }

        .metric {
          min-width: 0;
          min-height: 84px;
          display: flex;
          flex-direction: column;
          justify-content: space-between;
          padding: 11px 12px;
          border: 1px solid var(--line);
          border-radius: 8px;
          background: var(--surface);
          box-shadow: var(--shadow);
          text-align: left;
        }

        button.metric:hover { border-color: #b9c6d5; }
        .metric.active { border-color: var(--primary); box-shadow: inset 0 0 0 1px var(--primary); }
        .metric-label {
          min-height: 34px;
          display: flex;
          align-items: flex-start;
          color: var(--muted);
          font-size: 12px;
          line-height: 1.35;
          overflow-wrap: anywhere;
        }
        .metric-value { font-size: 24px; line-height: 1; font-weight: 760; color: #142036; }
        .metric.warning .metric-value { color: var(--amber); }
        .autoban-summary { grid-template-columns: repeat(6, minmax(132px, 1fr)); }
        .autoban-summary .metric-value { font-size: 23px; }

        .results { margin-top: 12px; overflow: hidden; }

        .results-head {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 12px;
          padding: 10px 12px;
          border-bottom: 1px solid var(--line);
        }

        .bulk-actions, .result-tools { display: flex; align-items: center; flex-wrap: wrap; gap: 7px; }
        .filter-context { color: var(--muted); font-size: 12px; }

        .progress-wrap { padding: 10px 12px; border-bottom: 1px solid var(--line); background: #fbfcfe; }
        .progress-line { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 7px; }
        .progress-copy { display: flex; align-items: center; gap: 7px; min-width: 0; font-weight: 660; color: #31568d; }
        .progress-copy span { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
        .progress-meta { color: var(--muted); font-size: 12px; white-space: nowrap; }
        .progress-track { height: 6px; border-radius: 5px; background: #e7edf5; overflow: hidden; }
        .progress-fill { width: 14.6%; height: 100%; border-radius: inherit; background: var(--primary); }

        .table-scroll { overflow-x: auto; }
        table { width: 100%; min-width: 970px; border-collapse: collapse; table-layout: fixed; }
        th { padding: 10px 12px; background: #f5f8fb; color: #5a6b82; font-size: 11px; text-align: left; }
        td { padding: 11px 12px; border-top: 1px solid var(--line); vertical-align: middle; color: #425269; }
        th:nth-child(1) { width: 25%; }
        th:nth-child(2), th:nth-child(3) { width: 10%; }
        th:nth-child(4) { width: 7%; }
        th:nth-child(5) { width: 9%; }
        th:nth-child(6) { width: 9%; }
        th:nth-child(7) { width: 20%; }
        th:nth-child(8) { width: 10%; }

        .account { color: #324965; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .badge {
          display: inline-flex;
          align-items: center;
          min-height: 24px;
          padding: 3px 8px;
          border-radius: 6px;
          font-size: 11px;
          font-weight: 720;
          white-space: nowrap;
        }
        .badge.green { color: var(--green); background: var(--green-soft); }
        .badge.amber { color: var(--amber); background: var(--amber-soft); }
        .badge.red { color: var(--red); background: var(--red-soft); }
        .badge.purple { color: var(--purple); background: var(--purple-soft); }
        .badge.blue { color: #315aa6; background: var(--primary-soft); }

        .row-actions { display: flex; align-items: center; gap: 5px; }
        .row-actions .icon-btn { width: 30px; height: 30px; border-color: var(--line); background: var(--surface); }
        .row-actions .danger-icon { color: var(--red); border-color: #f2d0d0; background: #fff8f8; }

        .table-footer {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 12px;
          padding: 10px 12px;
          border-top: 1px solid var(--line);
          color: var(--muted);
          font-size: 12px;
        }

        .pagination { display: flex; align-items: center; gap: 4px; }
        .table-footer-left { display: flex; align-items: center; flex-wrap: wrap; gap: 9px; }

        .autoban-table { min-width: 1080px; }
        .autoban-table th:nth-child(1) { width: 23%; }
        .autoban-table th:nth-child(2) { width: 13%; }
        .autoban-table th:nth-child(3) { width: 16%; }
        .autoban-table th:nth-child(4) { width: 18%; }
        .autoban-table th:nth-child(5) { width: 10%; }
        .autoban-table th:nth-child(6) { width: 10%; }
        .autoban-table th:nth-child(7) { width: 10%; }
        .autoban-table td:nth-child(3), .autoban-table td:nth-child(5) { white-space: nowrap; }
        .autoban-table .row-actions { justify-content: flex-start; }
        .autoban-table .restore-copy { color: var(--muted); font-size: 12px; }
        .autoban-table .sync-state { display: inline-flex; align-items: center; gap: 5px; color: var(--green); font-size: 12px; font-weight: 680; }
        .autoban-table .sync-state svg { width: 14px; height: 14px; }
        .page-btn {
          width: 30px;
          height: 30px;
          display: grid;
          place-items: center;
          border: 1px solid var(--line);
          border-radius: 6px;
          background: var(--surface);
          color: #506178;
        }
        .page-btn.active { border-color: var(--primary); color: #fff; background: var(--primary); }

        html[data-grok-theme="dark"] {
          color-scheme: dark;
          --bg: #0f151d;
          --surface: #171f2a;
          --surface-2: #111923;
          --line: #2a3543;
          --line-strong: #3b4858;
          --text: #edf2f8;
          --muted: #a6b1c1;
          --muted-2: #8190a4;
          --primary: #4b83ee;
          --primary-hover: #5b91f7;
          --primary-soft: #1a2c4c;
          --green: #5bd5ad;
          --green-soft: #17362f;
          --amber: #efba62;
          --amber-soft: #3a2b15;
          --red: #f27f7f;
          --red-soft: #3c2023;
          --purple: #b7a1ff;
          --purple-soft: #2d2448;
          --shadow: none;
        }

        html[data-grok-theme="dark"]
        html[data-grok-theme="dark"]
        html[data-grok-theme="dark"]
        html[data-grok-theme="dark"] .tab.active, .mode-tab.active { color: #c9ddff; border-color: #30476a; border-bottom-color: var(--primary-soft); }
        html[data-grok-theme="dark"] .tab.active small, .mode-tab.active small { color: #9eb6dd; }
        html[data-grok-theme="dark"] .sampling-row, html[data-grok-theme="dark"] .progress-wrap { background: #131b25; }
        html[data-grok-theme="dark"] .access-value { color: #b8c5d6; }
        html[data-grok-theme="dark"] .field, html[data-grok-theme="dark"] .check-control, html[data-grok-theme="dark"] .section-label { color: #c9d4e2; }
        html[data-grok-theme="dark"] .btn { color: #d8e2ee; background: var(--surface); border-color: var(--line-strong); }
        html[data-grok-theme="dark"] .btn:hover { color: #f4f7fb; background: #1d2835; border-color: #526176; }
        html[data-grok-theme="dark"] .btn.soft { color: #bfd5ff; background: var(--primary-soft); border-color: #35517c; }
        html[data-grok-theme="dark"] .btn.danger { color: #ffaaaa; background: var(--red-soft); border-color: #67383d; }
        html[data-grok-theme="dark"] .select-compact { color: #dce5f0; background: var(--surface); }
        html[data-grok-theme="dark"] th { color: #a9b7c9; background: #111923; }
        html[data-grok-theme="dark"] td { color: #bdc9d8; }
        html[data-grok-theme="dark"] .account { color: #dbe5f2; }
        html[data-grok-theme="dark"] .metric-value { color: #f4f7fb; }
        html[data-grok-theme="dark"] .autoban-heading-icon { color: #bcd4ff; }
        html[data-grok-theme="dark"] .toggle-track { background: #485568; }
        html[data-grok-theme="dark"] .autoban-table .restore-copy { color: #aeb9c8; }
        html[data-grok-theme="dark"] .row-actions .icon-btn, html[data-grok-theme="dark"] .page-btn { color: #c7d2df; background: var(--surface); border-color: var(--line-strong); }
        html[data-grok-theme="dark"] .row-actions .danger-icon { color: #ff9b9b; background: var(--red-soft); border-color: #67383d; }

        @media (max-width: 1220px) {
          .overview { grid-template-columns: repeat(4, minmax(132px, 1fr)); }
          .autoban-summary { grid-template-columns: repeat(3, minmax(132px, 1fr)); }
          .toolbar { grid-template-columns: 1fr; }
          .toolbar-actions { justify-content: flex-start; }
        }

        @media (max-width: 900px) {
          :root { --sidebar: 220px; }



          .overview { grid-template-columns: repeat(3, minmax(0, 1fr)); }
          .autoban-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); }
          .schedule-row { grid-template-columns: 1fr auto; }
          .schedule-controls { grid-column: 1 / -1; }
        }

        @media (max-width: 640px) {



          .page-head { align-items: flex-start; }
          h1 { font-size: 20px; }
          .head-actions { padding-top: 23px; }
          .head-actions .select-compact { width: 72px; }
          .panel { border-radius: 7px; }
          .access-row { align-items: flex-start; flex-direction: column; gap: 6px; }
          .access-value { width: 100%; min-width: 0; }
          .mode-tabs { display: grid; grid-template-columns: 1fr 1fr; padding: 8px 8px 0; }
          .tab, .mode-tab { min-width: 0; padding: 8px; align-items: flex-start; }
          .mode-tab svg { flex: 0 0 auto; margin-top: 2px; }
          .mode-tab > span { min-width: 0; }
          .tab small, .mode-tab small { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
          .toolbar { padding: 10px; }
          .toolbar-main, .toolbar-actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
          .toolbar .field, .toolbar .check-control, .toolbar .btn { width: 100%; min-width: 0; }
          .toolbar-actions .primary { grid-column: 1 / -1; }
          .sampling-row { display: block; padding: 10px; }
          .sampling-controls { display: grid; grid-template-columns: 1fr 1fr; margin-top: 8px; }
          .sampling-controls .field { width: 100%; min-width: 0; }
          .sampling-controls .btn { grid-column: 1 / -1; width: 100%; }
          .sample-summary { display: block; margin-top: 7px; white-space: normal; }
          .schedule-row { grid-template-columns: 1fr auto; padding: 10px; }
          .schedule-controls { display: grid; grid-template-columns: 1fr 1fr; }
          .schedule-controls .field, .schedule-controls .check-control { min-width: 0; width: 100%; }
          .schedule-controls .wide { grid-column: 1 / -1; }
          .overview { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 7px; }
          .autoban-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
          .metric { min-height: 82px; padding: 10px; }
          .metric-label { min-height: 33px; font-size: 11px; }
          .metric-value { font-size: 22px; }
          .results-head { align-items: flex-start; flex-direction: column; }
          .bulk-actions { display: grid; grid-template-columns: 1fr 1fr; width: 100%; }
          .bulk-actions .btn { width: 100%; min-width: 0; padding: 0 8px; }
          .result-tools { width: 100%; justify-content: space-between; }
          .progress-line { align-items: flex-start; flex-direction: column; gap: 4px; }
          .progress-copy { width: 100%; }
          .progress-meta { padding-left: 23px; }
          .table-footer { align-items: flex-start; flex-direction: column; }
          .table-footer-left { width: 100%; justify-content: space-between; }
          .pagination { width: 100%; justify-content: flex-end; }
          .autoban-bar { align-items: flex-start; flex-direction: column; gap: 10px; }
          .autoban-switch-row { width: 100%; justify-content: space-between; }
          .autoban-actions { align-items: flex-start; flex-direction: column; }
          .autoban-action-buttons { display: grid; grid-template-columns: 1fr 1fr; width: 100%; }
          .autoban-action-buttons .btn { width: 100%; min-width: 0; }
          .autoban-action-buttons .danger { grid-column: 1 / -1; }
          .autoban-pool-status { white-space: normal; }
        }

    /* Plugin host adaptations (no CPA chrome) */
    html, body { margin:0; min-height:0; min-width:0; }
    body {
      background: var(--bg) !important;
      color: var(--text) !important;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      font-size: 14px;
      line-height: 1.45;
      letter-spacing: 0;
    }
    .grok-inspection-page {
      width: min(1480px, calc(100% - 36px));
      margin: 0 auto;
      padding: 18px 0 42px;
      color: var(--text);
      min-width: 0;
    }
    .grok-inspection-page svg.ico, .grok-inspection-page .icon-btn svg, .grok-inspection-page .mode-tab svg,
    .grok-inspection-page .btn svg, .grok-inspection-page .section-label svg, .grok-inspection-page .key-state svg,
    .grok-inspection-page .autoban-heading-icon svg, .grok-inspection-page .progress-copy svg {
      width: 16px; height: 16px; stroke-width: 1.8; flex: 0 0 auto;
    }
    .sr-only { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip:rect(0,0,0,0); border:0; white-space:nowrap; }
    .access-row .access-value#managementKey, #managementKey.access-value {
      width: min(360px, 100%); min-width: 225px; flex: 1 1 auto; height: 32px;
      border: 1px solid var(--line); border-radius: 7px; padding: 0 10px;
      background: var(--surface-2); color: var(--text);
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    }
    #keyHint.key-state, .key-state#keyHint { color: var(--muted); font-weight: 650; }
    #keyHint.key-state.ok { color: var(--green); }
    .tab .tab-title, .mode-tab .tab-title { display:block; line-height:1.2; }
    .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small.tab-desc { display:block; margin-top:1px; color:var(--muted-2); font-size:11px; font-weight:520; }
    .tab.active .tab-desc, .mode-tab.active .tab-desc, .tab.active small, .mode-tab.active small.tab-desc { color:#5474a8; }
    button.mode-tab.tab, button.tab.tab, .mode-tab {
      font: inherit; cursor: pointer;
    }
    /* Keep real DOM panels for mode switching */
    #panel-inspect[hidden], #panel-autoban[hidden], .inspection-only[hidden], .autoban-only[hidden], [hidden] { display:none !important; }
    .help-popover[hidden] { display:none !important; opacity:0; visibility:hidden; }
    .help-popover.open { opacity:1; visibility:visible; transform:translateY(0); display:block; }
    .help-body p { margin:0; }
    .help-body p + p { margin-top:8px; }
    .help-body strong { color: var(--text); }
    /* Metric cards used by live summary renderer */
    .overview .metric, .summary .metric, button.metric, .card.metric {
      min-width:0; min-height:84px; display:flex; flex-direction:column; justify-content:space-between;
      padding:11px 12px; border:1px solid var(--line); border-radius:8px; background:var(--surface);
      box-shadow:var(--shadow); text-align:left; cursor:pointer; color:inherit;
    }
    button.metric:hover, .card.metric:hover { border-color:#b9c6d5; }
    .metric.active, .card.metric.active, .card.active {
      border-color: var(--primary); box-shadow: inset 0 0 0 1px var(--primary);
    }
    .metric-label, .metric .k, .card .k {
      min-height:34px; display:flex; align-items:flex-start; color:var(--muted);
      font-size:12px; line-height:1.35; overflow-wrap:break-word; word-break:normal;
    }
    .metric-value, .metric .v, .card .v { font-size:24px; line-height:1; font-weight:760; color:#142036; }
    .metric.warning .metric-value, .metric.warning .v { color: var(--amber); }
    .summary, .overview { display:grid; gap:8px; margin-top:12px; width:100%; min-width:0; }
    .summary, .overview.inspection-summary { grid-template-columns: repeat(7, minmax(132px, 1fr)); }
    .summary.ban-summary, .overview.autoban-summary, .autoban-summary {
      grid-template-columns: repeat(6, minmax(132px, 1fr));
    }
    .autoban-summary .metric-value, .ban-summary .metric-value, .ban-summary .v { font-size:23px; }
    /* Results shell */
    .results { margin-top:12px; overflow:hidden; }
    .results .table-wrap, .table-wrap.account-pool {
      border:0; box-shadow:none; border-radius:0; background:transparent; overflow:hidden;
    }
    .table-wrap.account-pool .table-scroll { overflow-x:auto; -webkit-overflow-scrolling:touch; }
    .table-wrap.account-pool .empty {
      min-height:140px; display:flex; align-items:center; justify-content:center; color:var(--muted);
      padding:48px 20px; text-align:center;
    }
    .table-wrap.account-pool table.inspect-table { min-width:970px; }
    .table-wrap.account-pool table.ban-table, .autoban-table { min-width:1080px; }
    table { width:100%; border-collapse:collapse; table-layout:fixed; font-size:13px; }
    th { padding:10px 12px; background:#f5f8fb; color:#5a6b82; font-size:11px; text-align:left; white-space:nowrap; }
    td { padding:11px 12px; border-top:1px solid var(--line); vertical-align:middle; color:#425269; }
    td.col-name, td.account { color:#324965; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    td.col-reason { white-space:normal; overflow-wrap:anywhere; word-break:break-word; }
    .badge, .pill {
      display:inline-flex; align-items:center; min-height:24px; padding:3px 8px; border-radius:6px;
      font-size:11px; font-weight:720; white-space:nowrap; flex-shrink:0;
    }
    .badge.green, .pill.green { color:var(--green); background:var(--green-soft); }
    .badge.amber, .pill.amber { color:var(--amber); background:var(--amber-soft); }
    .badge.red, .pill.red { color:var(--red); background:var(--red-soft); }
    .badge.purple, .pill.purple { color:var(--purple); background:var(--purple-soft); }
    .badge.blue, .pill.blue { color:#315aa6; background:var(--primary-soft); }
    .row-actions { display:flex; align-items:center; gap:5px; flex-wrap:wrap; }
    .row-actions .btn, .row-actions button {
      min-height:30px; height:30px; padding:0 8px; font-size:12px; border-radius:6px;
    }
    .progress.live, .progress-copy { color:#31568d; font-weight:660; }
    .progress-wrap { padding:10px 12px; border-bottom:1px solid var(--line); background:#fbfcfe; }
    .progress-wrap .progress { margin:0; }
    .progress-wrap .err { margin-top:6px; }
    .pager, .table-footer {
      display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap;
      padding:10px 12px; border-top:1px solid var(--line); color:var(--muted); font-size:12px;
      background: transparent;
    }
    .pager-meta, .pager-meta-row { color:var(--muted); font-size:12px; }
    .pager select, .select-compact, .grok-inspection-page select {
      height:34px; border:1px solid var(--line-strong); border-radius:7px; background:var(--surface);
      color:var(--text); padding:0 28px 0 10px; font-size:12px; color-scheme:inherit;
    }
    .ban-unsynced-banner { margin:8px 0 0; color:var(--amber); font-size:12px; }
    .err { color:var(--red); white-space:pre-wrap; font-size:12px; }
    /* Modal */
    .modal { position:fixed; inset:0; z-index:10050; display:flex; align-items:center; justify-content:center; background:rgba(15,23,42,.45); padding:16px; }
    .modal.hidden { display:none; }
    .modal-card { width:min(440px,100%); background:var(--surface); border-radius:12px; border:1px solid var(--line); box-shadow:0 20px 40px rgba(15,23,42,.18); padding:18px 18px 14px; color:var(--text); }
    .modal-title { font-size:16px; font-weight:700; margin-bottom:10px; }
    .modal-msg { font-size:13px; line-height:1.6; color:var(--muted); white-space:pre-wrap; margin-bottom:16px; }
    .modal-actions { display:flex; justify-content:flex-end; gap:8px; }
    /* Toggle switch maps existing markup */
    .switch, .toggle { position:relative; width:42px; height:24px; flex:0 0 auto; display:inline-block; }
    .switch input, .toggle input { position:absolute; width:1px; height:1px; opacity:0; pointer-events:none; }
    .slider, .toggle-track {
      position:absolute; inset:0; border-radius:12px; background:#b8c2cf; transition:.16s ease; cursor:pointer;
    }
    .slider:before, .toggle-track::after {
      content:''; position:absolute; top:3px; left:3px; width:18px; height:18px; border-radius:50%;
      background:#fff; box-shadow:0 1px 3px rgba(15,23,42,.28); transition:.16s ease;
    }
    .switch input:checked + .slider, .toggle input:checked + .toggle-track { background: var(--green); }
    .switch input:checked + .slider:before, .toggle input:checked + .toggle-track::after { transform: translateX(18px); }
    .status-pill {
      min-height:25px; display:inline-flex; align-items:center; padding:3px 8px; border-radius:6px;
      color:var(--green); background:var(--green-soft); font-size:11px; font-weight:730; white-space:nowrap;
    }
    .status-pill.off { color:var(--muted); background:var(--surface-2); }
    .status-pill.on { color:var(--green); background:var(--green-soft); }
    .autoban-heading-copy strong, .autoban-heading strong { display:block; font-size:15px; line-height:1.25; }
    .autoban-heading-copy small, .autoban-heading small, #banEnabledHint {
      display:block; margin-top:2px; color:var(--muted); font-size:11px;
    }
    .schedule-status, #scheduleStatus.schedule-status {
      display:inline-flex; align-items:center; gap:6px; color:var(--muted); font-size:12px; font-weight:650;
    }
    .status-dot { width:7px; height:7px; border-radius:50%; background:#a9b3c2; flex:0 0 auto; }
    tr.row-busy { opacity:.55; }
    tr.row-out { opacity:0; transform:translateX(8px); transition:opacity .28s ease, transform .28s ease; }
    /* Dark theme host attribute */
    html[data-grok-theme="dark"] {
      color-scheme: dark;
      --bg: #0f151d;
      --surface: #171f2a;
      --surface-2: #111923;
      --line: #2a3543;
      --line-strong: #3b4858;
      --text: #edf2f8;
      --muted: #a6b1c1;
      --muted-2: #8190a4;
      --primary: #4b83ee;
      --primary-hover: #5b91f7;
      --primary-soft: #1a2c4c;
      --green: #5bd5ad;
      --green-soft: #17362f;
      --amber: #efba62;
      --amber-soft: #3a2b15;
      --red: #f27f7f;
      --red-soft: #3c2023;
      --purple: #b7a1ff;
      --purple-soft: #2d2448;
      --shadow: none;
    }
    html[data-grok-theme="dark"] body, html[data-grok-theme="dark"] .grok-inspection-page {
      background: var(--bg) !important; color: var(--text) !important;
    }
    html[data-grok-theme="dark"] .metric-value, html[data-grok-theme="dark"] .card .v, html[data-grok-theme="dark"] .metric .v { color:#f4f7fb; }
    html[data-grok-theme="dark"] th { color:#a9b7c9; background:#111923; }
    html[data-grok-theme="dark"] td { color:#bdc9d8; }
    html[data-grok-theme="dark"] td.col-name, html[data-grok-theme="dark"] .account { color:#dbe5f2; }
    html[data-grok-theme="dark"] .sampling-row, html[data-grok-theme="dark"] .progress-wrap { background:#131b25; }
    html[data-grok-theme="dark"] .tab.active, .mode-tab.active { color:#c9ddff; border-color:#30476a; border-bottom-color:var(--primary-soft); }
    html[data-grok-theme="dark"] .tab.active .tab-desc, .mode-tab.active .tab-desc, html[data-grok-theme="dark"] .tab.active small, .mode-tab.active small { color:#9eb6dd; }
    html[data-grok-theme="dark"] .btn { color:#d8e2ee; background:var(--surface); border-color:var(--line-strong); }
    html[data-grok-theme="dark"] .btn.soft { color:#bfd5ff; background:var(--primary-soft); border-color:#35517c; }
    html[data-grok-theme="dark"] .btn.danger { color:#ffaaaa; background:var(--red-soft); border-color:#67383d; }
    html[data-grok-theme="dark"] .btn.primary { color:#fff; background:var(--primary); border-color:var(--primary); }
    html[data-grok-theme="dark"] .field, html[data-grok-theme="dark"] .check-control { color:#c9d4e2; background:var(--surface); border-color:var(--line); }
    html[data-grok-theme="dark"] #managementKey, html[data-grok-theme="dark"] .access-value { color:#b8c5d6; background:var(--surface-2); border-color:var(--line); }
    html[data-grok-theme="dark"] .slider, html[data-grok-theme="dark"] .toggle-track { background:#485568; }
    html[data-grok-theme="dark"] .status-pill.on { color:var(--green); background:var(--green-soft); }
    html[data-grok-theme="dark"] .status-pill.off { color:var(--muted); background:var(--surface-2); }
    html[data-grok-theme="dark"] .modal-card { background:var(--surface); border-color:var(--line); color:var(--text); }
    html[data-grok-theme="dark"] .modal-msg { color:var(--muted); }
    html[data-grok-theme="dark"] .eyebrow { color:#b9c4ff; background:#29315a; }
    html[data-grok-theme="dark"] .autoban-heading-icon { color:#bcd4ff; }
    @media (prefers-color-scheme: dark) {
      html:not([data-grok-theme="light"]) {
        color-scheme: dark;
        --bg: #0f151d; --surface:#171f2a; --surface-2:#111923; --line:#2a3543; --line-strong:#3b4858;
        --text:#edf2f8; --muted:#a6b1c1; --muted-2:#8190a4; --primary:#4b83ee; --primary-hover:#5b91f7;
        --primary-soft:#1a2c4c; --green:#5bd5ad; --green-soft:#17362f; --amber:#efba62; --amber-soft:#3a2b15;
        --red:#f27f7f; --red-soft:#3c2023; --purple:#b7a1ff; --purple-soft:#2d2448; --shadow:none;
      }
    }
    @media (max-width:1220px) {
      .summary, .overview.inspection-summary { grid-template-columns: repeat(4, minmax(132px, 1fr)); }
      .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns: repeat(3, minmax(132px, 1fr)); }
    }
    @media (max-width:900px) {
      .summary, .overview.inspection-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); }
      .summary.ban-summary, .overview.autoban-summary, .autoban-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); }
      .grok-inspection-page { width: min(100% - 24px, 1480px); padding-top:12px; }
    }
    @media (max-width:640px) {
      body { font-size:13px; overflow-x:hidden !important; }
      .grok-inspection-page { width:100%; padding:10px 10px 28px; }
      .summary, .overview.inspection-summary,
      .summary.ban-summary, .overview.autoban-summary, .autoban-summary {
        grid-template-columns: repeat(2, minmax(0, 1fr)) !important; gap:7px;
      }
      .mode-tabs { display:grid; grid-template-columns:1fr 1fr; padding:8px 8px 0; }
      .tab, .mode-tab { min-width:0; padding:8px; align-items:flex-start; width:100%; }
      .tab .tab-title, .mode-tab .tab-title, .tab .tab-desc, .mode-tab .tab-desc, .tab small, .mode-tab small {
        overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;
      }
      .toolbar { grid-template-columns:1fr; padding:10px; }
      .toolbar-main, .toolbar-actions { display:grid; grid-template-columns:repeat(2, minmax(0,1fr)); }
      .toolbar .field, .toolbar .check-control, .toolbar .btn { width:100%; min-width:0; }
      .toolbar-actions .primary, .toolbar-actions #runBtn { grid-column:1 / -1; }
      .sampling-row { display:block; padding:10px; }
      .sampling-controls { display:grid; grid-template-columns:1fr 1fr; margin-top:8px; }
      .sampling-controls .btn, .sampling-controls #sampleBtn { grid-column:1 / -1; width:100%; }
      .schedule-row { grid-template-columns:1fr auto; padding:10px; display:grid; gap:12px; }
      .schedule-controls { grid-column:1 / -1; display:grid; grid-template-columns:1fr 1fr; }
      .schedule-controls .field, .schedule-controls .check-control { min-width:0; width:100%; }
      .schedule-controls .wide { grid-column:1 / -1; }
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
      .access-row { flex-direction:column; align-items:flex-start; gap:6px; }
      #managementKey.access-value, .access-row .access-value { width:100%; min-width:0; }
      .table-wrap.account-pool table.inspect-table,
      .table-wrap.account-pool table.ban-table { min-width:720px !important; }
    }

    /* Contract: dark generic buttons must not restyle tabs */
    html[data-grok-theme="dark"] .grok-inspection-page button:not(.primary):not(.soft):not(.danger):not(.tab):not(.mode-tab):not(.icon-btn):not(.metric) {
      background: var(--surface);
      border-color: var(--line-strong);
      color: var(--text);
    }
    html[data-grok-theme="dark"] .grok-inspection-page select {
      background: var(--surface) !important;
      color: var(--text) !important;
      border-color: var(--line-strong) !important;
      color-scheme: dark;
    }
    .grok-inspection-page select { color-scheme: inherit; }
    /* Keep legacy mobile breakpoint marker used by host-theme tests; mirror 640 rules lightly */
    @media (max-width:760px) {
      body { overflow-x: hidden !important; }
      .grok-inspection-page { width: 100%; padding: 10px 10px 28px; }
      .mode-tabs, .tabs { display: grid !important; grid-template-columns: 1fr 1fr !important; width: 100%; }
      .tab, .mode-tab { width: 100%; min-width: 0; }
      .summary, .overview, .ban-summary, .autoban-summary { grid-template-columns: repeat(2, minmax(0, 1fr)) !important; }
      .autoban-bar { flex-direction: column; align-items: flex-start; }
      .autoban-actions { flex-direction: column; align-items: flex-start; }
      .table-wrap.account-pool table.inspect-table,
      .table-wrap.account-pool table.ban-table { min-width: 720px !important; }
    }


    /* host-theme mobile contract marker */
    @media (max-width:760px){ .grok-inspection-page .summary, .grok-inspection-page .overview { grid-template-columns:repeat(2,minmax(0,1fr)); } }

    .summary.ban-summary { grid-template-columns:repeat(6,minmax(0,1fr)); width:100%; min-width:0; }

    html[data-grok-theme="dark"] .grok-inspection-page .status-pill { color: var(--muted); }
    html[data-grok-theme="dark"] .grok-inspection-page .status-pill.on { color: var(--green); background: var(--green-soft); }

    html[data-grok-theme="dark"] .grok-inspection-page .tab.active { color:#c9ddff; background:var(--primary-soft); }

    html[data-grok-theme="dark"] .grok-inspection-page th { color:#a9b7c9; background:#111923; }

    @media (min-width:761px) and (max-width:1220px) {
      .summary, .overview.inspection-summary { grid-template-columns:repeat(4,minmax(0,1fr)); }
      .summary.ban-summary, .autoban-summary { grid-template-columns:repeat(3,minmax(0,1fr)); }
    }


    .page-head-main { min-width: 0; }
    .title-row { display: flex; align-items: center; gap: 7px; min-width: 0; }
    .head-actions { display: flex; align-items: center; gap: 8px; padding-top: 25px; flex: 0 0 auto; }
    .lang-ctl { display: inline-flex; align-items: center; margin: 0; }
    .help-wrap { position: relative; flex: 0 0 auto; }
    .help-btn { /* icon-btn alias */ }
    .autoban-bar {
      min-height: 58px; display: flex; align-items: center; justify-content: space-between; gap: 16px;
      padding: 11px 12px; border-bottom: 1px solid var(--line);
    }
    .autoban-actions {
      display: flex; align-items: center; justify-content: space-between; gap: 12px;
      padding: 10px 12px; flex-wrap: wrap;
    }


    .grok-inspection-page .tabs { display:flex; align-items:stretch; padding:10px 12px 0; gap:4px; }
`
