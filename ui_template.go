package main

const uiDocHead = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">Grok 账号巡检</title>
  <style>
`

const uiDocMid = `  </style>
</head>
<body>
  <div class="wrap grok-inspection-page">
    <div class="page-head">
      <div class="page-head-main">
        <div class="eyebrow badge">xAI / Grok · CPA Plugin</div>
        <div class="title-row">
          <h1 data-i18n="title">Grok 账号巡检</h1>
          <div class="help-wrap">
            <button type="button" class="icon-btn help-btn" id="helpBtn" data-i18n-title="help_title" data-i18n-aria-label="help_title" title="巡检说明" aria-label="巡检说明" aria-expanded="false" aria-controls="helpPopover"><svg class="ico" data-icon="circle-help" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/></svg></button>
            <div class="help-popover" id="helpPopover" role="dialog" aria-modal="false" hidden>
              <div id="heroSub" class="help-body"></div>
            </div>
          </div>
        </div>
      </div>
      <div class="head-actions">
        <label class="lang-ctl">
          <span class="sr-only" data-i18n="language">语言</span>
          <select id="langSelect" class="select-compact" data-i18n-aria-label="language" aria-label="语言">
            <option value="zh">中文</option>
            <option value="en">EN</option>
          </select>
        </label>
      </div>
    </div>

    <section class="panel controls-panel" aria-label="Inspection controls">
      <div class="access-row shared-key" id="keyRow">
        <input id="managementKey" class="access-value" type="password" autocomplete="current-password" data-i18n-placeholder="key_label" placeholder="CPA Management Key（可自动读取管理面板）">
        <span class="hint" id="keyHint"></span>
      </div>

      <div class="mode-tabs tabs" role="tablist" aria-label="功能页签" data-i18n-aria-label="tabs_aria">
        <button class="tab active" type="button" data-tab="inspect" id="tabInspect" aria-selected="true" role="tab">
          <svg class="ico" data-icon="scan-search" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7V5a2 2 0 0 1 2-2h2"/><path d="M17 3h2a2 2 0 0 1 2 2v2"/><path d="M21 17v2a2 2 0 0 1-2 2h-2"/><path d="M7 21H5a2 2 0 0 1-2-2v-2"/><circle cx="12" cy="12" r="3"/><path d="m16 16-1.5-1.5"/></svg>
          <span><span class="tab-title" data-i18n="tab_inspect">账号巡检</span><small class="tab-desc" data-i18n="tab_inspect_desc">批量探测 · 建议操作</small></span>
        </button>
        <button class="tab" type="button" data-tab="autoban" id="tabAutoban" aria-selected="false" role="tab">
          <svg class="ico" data-icon="shield-ban" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 13c0 5-3.5 7.5-8 9-4.5-1.5-8-4-8-9V5l8-3 8 3z"/><path d="m4.9 4.9 14.2 14.2"/></svg>
          <span><span class="tab-title" data-i18n="tab_autoban">实时自动禁用</span><small class="tab-desc" data-i18n="tab_autoban_desc">请求拦截 · 定时恢复</small></span>
        </button>
      </div>

      <div class="panel active panel-inspect-body inspection-only" id="panel-inspect">
        <div class="toolbar">
          <div class="toolbar-main">
            <div class="field"><label for="workers" data-i18n="workers">并发</label><input id="workers" type="number" min="1" max="16" step="1" value="6" data-i18n-title="workers_title" title="1-16 的整数"></div>
            <label class="check-control"><input id="includeDisabled" type="checkbox"><span data-i18n="include_disabled">包含已禁用</span></label>
            <label class="check-control"><input id="onlyDisabled" type="checkbox"><span data-i18n="only_disabled">仅巡检已禁用</span></label>
          </div>
          <div class="toolbar-actions">
            <button id="stopBtn" class="btn" type="button" disabled><svg class="ico" data-icon="square" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="18" height="18" x="3" y="3" rx="2"/></svg><span data-i18n="stop">停止</span></button>
            <button id="applyBtn" class="btn soft" type="button" disabled><svg class="ico" data-icon="wand" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 4V2"/><path d="M15 16v-2"/><path d="M8 9h2"/><path d="M20 9h2"/><path d="M17.8 11.8 19 13"/><path d="M15 9h.01"/><path d="M17.8 6.2 19 5"/><path d="m3 21 9-9"/><path d="M12.2 6.2 11 5"/></svg><span data-i18n="apply_suggested">执行建议操作</span></button>
            <button id="incrBtn" class="btn soft" type="button" disabled data-i18n-title="incremental_title" title="只检测 Auth 中相对上次结果新增的账号"><svg class="ico" data-icon="list-plus" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M11 12H3"/><path d="M16 6H3"/><path d="M16 18H3"/><path d="M18 9v6"/><path d="M21 12h-6"/></svg><span data-i18n="incremental">增量巡检</span></button>
            <button id="filterRunBtn" class="btn soft" type="button" disabled data-i18n-title="category_title" title="只重新探测当前卡片筛选分类下的账号，保留其他结果"><svg class="ico" data-icon="scan-line" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7V5a2 2 0 0 1 2-2h2"/><path d="M17 3h2a2 2 0 0 1 2 2v2"/><path d="M21 17v2a2 2 0 0 1-2 2h-2"/><path d="M7 21H5a2 2 0 0 1-2-2v-2"/><path d="M7 12h10"/></svg><span data-i18n="inspect_category">巡检当前分类</span></button>
            <button id="runBtn" class="btn primary" type="button"><svg class="ico" data-icon="play" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polygon points="6 3 20 12 6 21 6 3"/></svg><span data-i18n="start">开始巡检</span></button>
          </div>
        </div>

        <div class="sampling-row sample-controls" id="samplingRow">
          <span class="section-label"><svg class="ico" data-icon="dices" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="12" height="12" x="2" y="10" rx="2"/><path d="m17.92 14 3.5-3.5a2.24 2.24 0 0 0 0-3l-5-4.92a2.24 2.24 0 0 0-3 0L10 6"/><path d="M6 18h.01"/><path d="M10 14h.01"/><path d="M15 6h.01"/><path d="M18 9h.01"/></svg><span data-i18n="sample_run">抽检</span></span>
          <div class="sampling-controls">
            <div class="field"><label for="sampleCount" data-i18n="sample_count">抽检数量</label><input id="sampleCount" type="number" min="0" step="1" value="" data-i18n-title="sample_count_title" title="从当前巡检范围随机抽取的账号数量，0 表示不按数量限制"></div>
            <div class="field"><label for="samplePercent" data-i18n="sample_percent">抽检比例%</label><input id="samplePercent" type="number" min="0" max="100" step="1" value="" data-i18n-title="sample_percent_title" title="从当前巡检范围按百分比抽取；数量与比例都填时取更小值"></div>
            <button id="sampleBtn" class="btn soft" type="button" disabled data-i18n-title="sample_title" title="按数量/比例随机抽检当前范围，不清空历史结果"><svg class="ico" data-icon="shuffle" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m18 14 4 4-4 4"/><path d="m18 2 4 4-4 4"/><path d="M2 18h1.973a4 4 0 0 0 3.3-1.7l5.454-8.6a4 4 0 0 1 3.3-1.7H22"/><path d="M2 6h1.972a4 4 0 0 1 3.6 2.2"/><path d="M22 18h-6.041a4 4 0 0 1-3.3-1.8l-.359-.45"/></svg><span data-i18n="sample_run">抽检</span></button>
          </div>
        </div>

        <div class="schedule-row">
          <span id="scheduleStatus" class="schedule-status">自动巡检状态加载中…</span>
          <div class="schedule-actions">
            <div class="schedule-controls">
              <label class="check-control"><input id="scheduleEnabled" type="checkbox"><span data-i18n="schedule_enabled">自动巡检</span></label>
              <div class="field"><label for="scheduleInterval" data-i18n="schedule_interval">间隔（分钟）</label><input id="scheduleInterval" type="number" min="1" max="10080" step="1" value="60"></div>
              <div class="field"><label for="scheduleWorkers" data-i18n="schedule_workers">并发</label><input id="scheduleWorkers" type="number" min="1" max="16" step="1" value="6"></div>
              <label class="check-control"><input id="scheduleIncludeDisabled" type="checkbox"><span data-i18n="schedule_include_disabled">包含已禁用</span></label>
              <div class="field wide"><label for="scheduleScope" data-i18n="schedule_scope">巡检范围</label>
              <select id="scheduleScope" data-i18n-title="schedule_scope_title" title="全量检测所有账号；抽检沿用上方「抽检」行的数量与比例">
                <option value="full" data-i18n="schedule_scope_full">全量</option>
                <option value="sample" data-i18n="schedule_scope_sample">抽检</option>
              </select>
              </div>
              <div class="field wide"><label for="schedule403Action" data-i18n="schedule_403_action">403 处理</label>
              <select id="schedule403Action">
                <option value="disable" data-i18n="schedule_action_disable">禁用</option>
                <option value="delete" data-i18n="schedule_action_delete">删除</option>
              </select>
              </div>
              <div class="field wide"><label for="schedule402Action" data-i18n="schedule_402_action">402 处理</label>
              <select id="schedule402Action">
                <option value="disable" data-i18n="schedule_action_disable">禁用</option>
                <option value="delete" data-i18n="schedule_action_delete">删除</option>
              </select>
              </div>
            </div>
            <button id="scheduleSaveBtn" class="btn soft" type="button"><svg class="ico" data-icon="save" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/><path d="M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7"/><path d="M7 3v4a1 1 0 0 0 1 1h7"/></svg><span data-i18n="schedule_save">保存自动巡检</span></button>
          </div>
        </div>
      </div>
    </section>

    <section class="autoban-control" id="panel-autoban" aria-label="Realtime auto-ban controls" hidden>
      <div class="autoban-bar">
        <div class="autoban-heading">
          <span class="autoban-heading-icon"><svg class="ico" data-icon="shield-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 13c0 5-3.5 7.5-8 9-4.5-1.5-8-4-8-9V5l8-3 8 3z"/><path d="m9 12 2 2 4-4"/></svg></span>
          <div class="autoban-heading-copy">
            <strong data-i18n="ban_title">实时自动禁用</strong>
            <small id="banEnabledHint" data-i18n="ban_enabled_hint">开关会立即生效并保存</small>
          </div>
        </div>
        <div class="autoban-switch-row switch-row">
          <label class="switch toggle" data-i18n-title="ban_enable" title="开启后实时拦截并禁用">
            <input id="banEnabledToggle" type="checkbox">
            <span class="slider toggle-track"></span>
          </label>
          <span id="banEnabledPill" class="status-pill off" data-i18n="ban_off">已关闭</span>
        </div>
      </div>
      <div class="autoban-actions">
        <div class="autoban-action-buttons">
          <button id="banRefreshBtn" class="btn" type="button"><svg class="ico" data-icon="refresh-cw" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg><span data-i18n="ban_refresh">刷新状态</span></button>
          <button id="banUnbanFilterBtn" class="btn soft" type="button" disabled><svg class="ico" data-icon="unlock" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/></svg><span data-i18n="ban_unban_filter">解禁当前分类</span></button>
          <button id="banUnbanAllBtn" class="btn danger" type="button"><svg class="ico" data-icon="shield-off" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m2 2 20 20"/><path d="M5 5a1 1 0 0 0-.5.1l-1.5.7v7.2c0 5 3.5 7.5 8 9 1.3-.4 2.4-1 3.3-1.7"/><path d="M19.7 14c.2-.7.3-1.4.3-2.1V5l-8-3-3.2 1.2"/></svg><span data-i18n="ban_unban_all">全部解禁</span></button>
        </div>
        <span class="hint autoban-pool-status" id="banFilterHint" data-i18n="ban_filter_hint">点击下方卡片筛选分类</span>
      </div>
    </section>

    <div id="banUnsyncedBanner" class="hint ban-unsynced-banner autoban-only" style="display:none" hidden></div>

    <div id="summary" class="overview summary inspection-only inspection-summary" aria-label="Inspection summary"></div>

    <section class="panel results inspection-only inspection-results" aria-label="Inspection results">
      <div class="results-head">
        <div class="bulk-actions actions-row">
          <button id="batchExportBtn" class="btn" type="button" disabled><svg class="ico" data-icon="download" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 15V3"/><path d="m7 10 5 5 5-5"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/></svg><span data-i18n="bulk_export">批量导出</span></button>
          <button id="batchDisableBtn" class="btn soft" type="button" disabled><svg class="ico" data-icon="ban" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="m4.9 4.9 14.2 14.2"/></svg><span data-i18n="bulk_disable">批量禁用</span></button>
          <button id="batchEnableBtn" class="btn soft" type="button" disabled><svg class="ico" data-icon="circle-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></svg><span data-i18n="bulk_enable">批量启用</span></button>
          <button id="batchDeleteBtn" class="btn danger" type="button" disabled><svg class="ico" data-icon="trash-2" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/><line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/></svg><span data-i18n="bulk_delete">批量删除</span></button>
        </div>
        <div class="result-tools">
          <span class="filter-context hint" id="exportHint" data-i18n="filter_hint">点击上方卡片切换分类；禁用/启用数量按当前分类下列表的启用/禁用状态统计</span>
        </div>
      </div>
      <div class="progress-wrap bar">
        <div id="progress" class="progress" data-i18n="waiting">等待开始</div>
        <pre id="error" class="err" style="margin:0;max-width:100%;text-align:left;font-size:12px;line-height:1.45;white-space:pre-wrap;word-break:break-word"></pre>
      </div>
      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="inspect-table">
            <thead>
              <tr>
                <th class="col-name" data-i18n="th_account">账号</th>
                <th class="col-status" data-i18n="th_status">当前状态</th>
                <th class="col-result" data-i18n="th_result">检测结果</th>
                <th class="col-http" data-i18n="th_http">HTTP</th>
                <th class="col-model" data-i18n="th_model">模型</th>
                <th class="col-action" data-i18n="th_action">建议</th>
                <th class="col-reason" data-i18n="th_reason">原因</th>
                <th class="col-ops" data-i18n="th_ops">操作</th>
              </tr>
            </thead>
            <tbody id="rows"></tbody>
          </table>
        </div>
        <div id="empty" class="empty" data-i18n="need_key_load">请输入 CPA Management Key 后加载巡检状态</div>
        <div id="pager" class="table-footer pager"></div>
      </div>
    </section>

    <div id="banSummary" class="overview summary ban-summary autoban-summary autoban-only" aria-label="Auto-ban summary" hidden>
      <div class="metric card active" data-ban-filter="all"><span class="metric-label k" data-i18n="ban_all">全部</span><strong class="metric-value v" id="banCount">0</strong></div>
      <div class="metric card" data-ban-filter="quota"><span class="metric-label k" data-i18n="ban_quota">额度用尽</span><strong class="metric-value v" id="banQuotaCount">0</strong></div>
      <div class="metric card" data-ban-filter="spending_limit"><span class="metric-label k" data-i18n="ban_spending_limit">402 额度受限</span><strong class="metric-value v" id="banSpendingLimitCount">0</strong></div>
      <div class="metric card" data-ban-filter="permission"><span class="metric-label k" data-i18n="ban_permission">权限拒绝</span><strong class="metric-value v" id="banPermissionCount">0</strong></div>
      <div class="metric card" data-ban-filter="unauthorized"><span class="metric-label k" data-i18n="ban_authfail">401 认证失败</span><strong class="metric-value v" id="banUnauthorizedCount">0</strong></div>
      <div class="metric card" data-ban-filter="manual"><span class="metric-label k" data-i18n="ban_manual_disabled">手动禁用</span><strong class="metric-value v" id="banManualDisabledCount">0</strong></div>
    </div>

    <section class="panel results autoban-only autoban-results" aria-label="Auto-ban account pool" hidden>
      <div class="results-head">
        <div class="bulk-actions autoban-pool-title">
          <span class="section-label"><svg class="ico" data-icon="shield-ban" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 13c0 5-3.5 7.5-8 9-4.5-1.5-8-4-8-9V5l8-3 8 3z"/><path d="m4.9 4.9 14.2 14.2"/></svg><span data-i18n="ban_title">实时自动禁用</span></span>
        </div>
        <div class="result-tools">
          <span class="filter-context autoban-filter-context" id="banPoolContext"></span>
        </div>
      </div>
      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="ban-table autoban-table">
            <thead>
              <tr>
                <th class="col-name" data-i18n="th_account">账号</th>
                <th data-i18n="ban_th_reason">禁用原因</th>
                <th data-i18n="ban_th_time">禁用时间</th>
                <th data-i18n="ban_th_restore">恢复方式</th>
                <th data-i18n="ban_th_remain">剩余</th>
                <th data-i18n="ban_th_sync">CPA 同步</th>
                <th class="col-ops" data-i18n="th_ops">操作</th>
              </tr>
            </thead>
            <tbody id="banRows"></tbody>
          </table>
        </div>
        <div id="banEmpty" class="empty" data-i18n="ban_status_loading">加载中…</div>
        <div id="banPager" class="table-footer pager"></div>
      </div>
      <pre id="banError" class="err" style="margin:10px 12px;font-size:12px;white-space:pre-wrap"></pre>
    </section>

    <div id="confirmModal" class="modal hidden" aria-hidden="true">
      <div class="modal-card" role="dialog" aria-modal="true">
        <div id="confirmTitle" class="modal-title" data-i18n="confirm_title">确认操作</div>
        <div id="confirmMsg" class="modal-msg"></div>
        <div class="modal-actions">
          <button type="button" class="btn" id="confirmCancel" data-i18n="cancel">取消</button>
          <button type="button" class="btn primary" id="confirmOk" data-i18n="ok">确定</button>
        </div>
      </div>
    </div>
  </div>
  <script>
`

const uiDocTail = `</script>
</body>
</html>`
