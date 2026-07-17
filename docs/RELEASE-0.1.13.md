## grok-inspection v0.1.13

### 摘要

双网关巡检 + 一键切换官方 API（`using_api` + `base_url`），在 CLI chat-proxy 拒绝但 `api.x.ai` 仍可用时**无需重登**即可恢复原生 `grok-4.5` 对话能力（需 CPA 支持 `using_api` 语义的版本）。

### 主要变更

1. **双通道探测**
   - 优先：`cli-chat-proxy.grok.com`（`/v1/responses`，必要时 fallback `/v1/chat/completions`）
   - 当 CLI 为权限拒绝（如 402/403 / `permission-denied`）时，再用**同一 access_token** 探测  
     `https://api.x.ai/v1/chat/completions`（精简头，无 CLI 专用头）
   - **不**用 API 结果掩盖 401/reauth；额度用尽不切网关

2. **新分类 / 动作**
   - `api_gateway_ok` + `switch_base_url`：CLI 拒、官方 API 通，提示无需重登
   - 结果字段：`cli_http_status`、`api_http_status`、`preferred_base_url`、`auth_base_url`、`using_api`、`gateway_note` 等

3. **一键应用** `POST .../baseurl/apply`
   - 对 `api_gateway_ok`（或指定 indexes）写入：
     - `using_api: true`
     - `base_url: https://api.x.ai/v1`
   - **不改 token**；UI 确认说明依赖 CPA 的 `using_api` 路由语义

4. **UI**
   - 双状态展示、「API 可用」卡片、应用 base_url 按钮与进度
   - 筛选标签支持 `api_gateway_ok`

5. **工程**
   - `host_stub.go`（`//go:build !cgo`）便于无 CGO 单测
   - 文档：README / README.en / ARCHITECTURE 同步至 0.1.13
   - openai-compat 自动同步仍作**文档后备**（本版以原生 `using_api` 为主路径）

### 依赖 / 说明

| 场景 | 说明 |
|------|------|
| CPA 已含 `using_api` 语义（本配套 CPA PR / 自定义构建） | 应用后原生 `grok-4.5` 日志应为 `source=DefaultAPIBaseURL` |
| 旧 CPA | 文件字段已写入；chat 可能仍被 rewrite 到 CLI，需升级 CPA 或临时用 openai-compat 后备 |
| 已禁用且双端 403 | 实测多为账号侧真正不可用，非误判；仅 CLI 拒且 API 2xx 才会标 `api_gateway_ok` |

### 测试

- [x] 本地 `go test`（含 baseurl / host stub）
- [x] linux/amd64 `.so` 部署现网，插件加载 0.1.13
- [x] 现网：`using_api=true` 后 CPA 日志走 `api.x.ai`，chat 200
- [x] 抽样禁用账号双探：多为 CLI/API 双 403（真实不可用）

### 版本

`pluginVersion = 0.1.13`
