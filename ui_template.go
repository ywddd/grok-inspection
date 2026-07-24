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
    <div class="hero">
      <div>
        <div class="badge">xAI / Grok · CPA Plugin</div>
        <h1 data-i18n="title">Grok 账号巡检</h1>
        <p class="sub" id="heroSub" data-i18n="subtitle">「开始巡检」清空并重测全部；「增量巡检」只测新增账号；「抽检」按数量/比例随机抽测并保留其他结果；「巡检当前分类」只重测所选分类（需先点分类卡片）；「批量操作」只作用于当前筛选；结果会自动保存。</p>
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
      <div class="key-row" style="flex:1;min-width:min(360px,100%)">
        <input id="managementKey" type="password" autocomplete="current-password" data-i18n-placeholder="key_label" placeholder="CPA Management Key（可自动读取管理面板）">
        <span class="hint" id="keyHint"></span>
      </div>
    </div>
    <div class="tabs" role="tablist" aria-label="功能页签" data-i18n-aria-label="tabs_aria">
      <button class="tab active" type="button" data-tab="inspect" id="tabInspect" aria-selected="true" role="tab"><span class="tab-title" data-i18n="tab_inspect">账号巡检</span><span class="tab-desc" data-i18n="tab_inspect_desc">批量探测 · 建议操作</span></button>
      <button class="tab" type="button" data-tab="autoban" id="tabAutoban" aria-selected="false" role="tab"><span class="tab-title" data-i18n="tab_autoban">实时自动禁用</span><span class="tab-desc" data-i18n="tab_autoban_desc">请求拦截 · 定时恢复</span></button>

    </div>
    <section class="panel active" id="panel-inspect">
    <div class="controls">
      <label class="ctl"><span data-i18n="workers">并发</span> <input id="workers" type="number" min="1" max="16" step="1" value="6" data-i18n-title="workers_title" title="1-16 的整数"></label>
      <label class="ctl"><input id="includeDisabled" type="checkbox"> <span data-i18n="include_disabled">包含已禁用</span></label>
      <label class="ctl"><input id="onlyDisabled" type="checkbox"> <span data-i18n="only_disabled">仅巡检已禁用</span></label>
      <button id="stopBtn" disabled data-i18n="stop">停止</button>
      <button id="applyBtn" class="soft" disabled data-i18n="apply_suggested">执行建议操作</button>
      <button id="incrBtn" class="soft" disabled data-i18n-title="incremental_title" title="只检测 Auth 中相对上次结果新增的账号" data-i18n="incremental">增量巡检</button>
      <button id="filterRunBtn" class="soft" disabled data-i18n-title="category_title" title="只重新探测当前卡片筛选分类下的账号，保留其他结果" data-i18n="inspect_category">巡检当前分类</button>
      <button id="runBtn" class="primary" data-i18n="start">开始巡检</button>
    </div>
    <div class="controls sample-controls">
      <label class="ctl"><span data-i18n="sample_count">抽检数量</span> <input id="sampleCount" type="number" min="0" step="1" value="" data-i18n-title="sample_count_title" title="从当前巡检范围随机抽取的账号数量，0 表示不按数量限制"></label>
      <label class="ctl"><span data-i18n="sample_percent">抽检比例%</span> <input id="samplePercent" type="number" min="0" max="100" step="1" value="" data-i18n-title="sample_percent_title" title="从当前巡检范围按百分比抽取，0 表示不按比例限制；若数量与比例都填则取更小值"></label>
      <button id="sampleBtn" class="soft" disabled data-i18n-title="sample_title" title="按数量/比例随机抽检当前范围，不清空历史结果" data-i18n="sample_run">抽检</button>
    </div>
    <div class="schedule-row">
      <label class="ctl"><input id="scheduleEnabled" type="checkbox"> <span data-i18n="schedule_enabled">自动巡检</span></label>
      <label class="ctl"><span data-i18n="schedule_interval">间隔（分钟）</span> <input id="scheduleInterval" type="number" min="1" max="10080" step="1" value="60"></label>
      <label class="ctl"><span data-i18n="schedule_workers">并发</span> <input id="scheduleWorkers" type="number" min="1" max="16" step="1" value="6"></label>
      <label class="ctl"><input id="scheduleIncludeDisabled" type="checkbox"> <span data-i18n="schedule_include_disabled">包含已禁用</span></label>
      <label class="ctl"><span data-i18n="schedule_403_action">403 处理</span>
        <select id="schedule403Action">
          <option value="disable" data-i18n="schedule_action_disable">禁用</option>
          <option value="delete" data-i18n="schedule_action_delete">删除</option>
        </select>
      </label>
      <label class="ctl"><span data-i18n="schedule_402_action">402 处理</span>
        <select id="schedule402Action">
          <option value="disable" data-i18n="schedule_action_disable">禁用</option>
          <option value="delete" data-i18n="schedule_action_delete">删除</option>
        </select>
      </label>
      <button id="scheduleSaveBtn" class="soft" type="button" data-i18n="schedule_save">保存自动巡检</button>
      <span id="scheduleStatus" class="schedule-status" data-i18n="schedule_loading">自动巡检状态加载中…</span>
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
      <div style="display:flex;flex-direction:column;align-items:flex-end;gap:4px;min-width:0;max-width:100%">
        <div id="progress" class="progress" data-i18n="waiting">等待开始</div>
        <pre id="error" class="err" style="margin:0;max-width:min(720px,100%);text-align:left;font-size:12px;line-height:1.45;white-space:pre-wrap;word-break:break-word"></pre>
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
      <div id="banUnsyncedBanner" class="hint" style="display:none;margin-bottom:8px;color:var(--warn,#b45309)"></div>
      <div id="banSummary" class="summary ban-summary">
        <div class="card active" data-ban-filter="all"><div class="k" data-i18n="ban_all">全部</div><div class="v" id="banCount">0</div></div>
        <div class="card" data-ban-filter="quota"><div class="k" data-i18n="ban_quota">额度用尽</div><div class="v" id="banQuotaCount">0</div></div>
        <div class="card" data-ban-filter="spending_limit"><div class="k" data-i18n="ban_spending_limit">402 额度受限</div><div class="v" id="banSpendingLimitCount">0</div></div>
        <div class="card" data-ban-filter="permission"><div class="k" data-i18n="ban_permission">权限拒绝</div><div class="v" id="banPermissionCount">0</div></div>
        <div class="card" data-ban-filter="unauthorized"><div class="k" data-i18n="ban_authfail">401 认证失败</div><div class="v" id="banUnauthorizedCount">0</div></div>
        <div class="card" data-ban-filter="manual"><div class="k" data-i18n="ban_manual_disabled">手动禁用</div><div class="v" id="banManualDisabledCount">0</div></div>
      </div>
      <div class="table-wrap account-pool">
        <div class="table-scroll">
          <table class="ban-table">
            <thead>
              <tr>
                <th class="col-name" data-i18n="th_account">账号</th><th data-i18n="ban_th_reason">禁用原因</th><th data-i18n="ban_th_time">禁用时间</th><th data-i18n="ban_th_restore">恢复方式</th><th data-i18n="ban_th_remain">剩余</th><th data-i18n="ban_th_sync">CPA 同步</th><th class="col-ops" data-i18n="th_ops">操作</th>
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
`

const uiDocTail = `</script>
</body>
</html>`
