# Grok Inspection

> **中文** | [English](README.en.md)

CPA（CLIProxyAPI）插件：批量检测 xAI / Grok 账号健康状态，并给出禁用 / 启用 / 删除建议。

## 说明

这是一个 **纯 vibe coding** 产物：功能能跑、够用，但代码风格、结构和边界细节未必按传统工程规范打磨。

- **欢迎提 Issue / PR**：修 bug、补能力、改体验、重构都欢迎。
- **如果你介意 vibe coding**：不必强用本插件，可以直接用 **CPA Manager Plus** 等管理面板里更完整的账号巡检 / 运维能力作为替代。
- 本插件定位是轻量、**可选**的 Grok/xAI 巡检补充，不是官方标准实现。

版本：`0.1.14` · 菜单：**Grok 账号巡检**

## 语言
管理界面支持 **中英双语切换**：
- **中文（默认）** — 本项目的第一语言
- **English** — 页面顶部 **语言 / Language** 选择器可切换
- 选择会保存在 `localStorage`（键名 `grok-inspection.lang`），刷新后恢复
- 开始巡检时会把所选 `lang` 传给后端，使原因/错误文案与界面语言一致
English docs: [README.en.md](README.en.md).
## 功能

- 完整巡检、增量巡检、按分类重巡 Grok/xAI 账号
- 识别健康、权限被拒、额度用尽、需重新登录、模型不可用、探测异常
- 后台执行巡检和批量操作，切换页面不丢任务
- 支持一键执行建议、批量禁用、批量删除、单账号处理
- 巡检结果落盘，页面重新打开后可恢复
- 支持导出当前筛选结果为 JSON/TXT
- 实时自动禁用（默认开启）：额度用尽 24h 后自动恢复；403/401 需手动解禁

## 安装

从 [Releases](https://github.com/ywddd/grok-inspection/releases) 下载与你的 CPA 平台匹配的压缩包：

| 平台 | 文件 |
|------|------|
| Linux amd64 | `grok-inspection_*_linux_amd64.zip` |
| Linux arm64 | `grok-inspection_*_linux_arm64.zip` |
| Windows amd64 | `grok-inspection_*_windows_amd64.zip` |
| macOS arm64 | `grok-inspection_*_darwin_arm64.zip` |

解压后把插件文件放到 CPA 插件目录：

```text
grok-inspection.so      # Linux
grok-inspection.dll     # Windows
grok-inspection.dylib   # macOS
```

在 CPA 配置中启用插件：

```yaml
plugins:
  enabled: true
  configs:
    grok-inspection:
      enabled: true
      priority: 1
```

重启 CPA 后，在管理页面打开 **Grok 账号巡检**，输入 CPA Management Key 即可使用。

## Docker

如果 CPA 运行在 Docker 中，把插件拷到容器内的插件目录后重启容器。容器名和插件路径以你的实际环境为准，例如：

```bash
docker cp ./grok-inspection.so <容器名>:<插件目录>/grok-inspection.so
docker restart <容器名>
```

删除 / 禁用账号时，会使用页面里填写的 CPA Management Key。

插件默认通过本机回环地址调用 CPA Management API。在 Docker、端口映射或自定义监听端口环境中，如果插件无法访问实际管理端口，请在 CPA 进程中显式设置：

```bash
CPA_MANAGEMENT_BASE_URL=http://127.0.0.1:<实际端口>
```

启用 TLS 时使用 `https://`。显式配置后，请求失败也不会回退到浏览器请求的 Origin。

## 使用

1. 打开 **Grok 账号巡检**。
2. 输入 CPA Management Key。
3. 选择并发数、是否包含已禁用账号、是否仅巡检已禁用账号。
4. 点击 **开始巡检** 或 **增量巡检**。
5. 根据结果执行 **一键建议操作**、批量禁用、批量删除或单账号操作。

巡检和批量操作都在后台执行。页面关闭或切换后任务仍会继续；重新打开页面可查看当前进度和上次结果。

点 **停止** 会立即结束本轮巡检：尚未探测的账号会标记为「已停止，未探测」。

## 结果说明

| 结果 | 默认建议 | 含义 |
|------|----------|------|
| 健康 | 保留；如果已禁用则建议启用 | 对话探测成功，账号可用 |
| 权限被拒 | 禁用 | 账号没有对话权限、被拒绝或权限受限 |
| 额度用尽 | 禁用 | 免费额度用尽，暂时不适合继续调度 |
| 需重新登录 | 删除 | 登录态失效，删除后需在 CPA 重新登录 |
| 模型不可用 | 保留 | 当前探测模型不可用，不一定代表账号失效 |
| 探测异常 | 保留 | 网络或上游异常，建议复查后再处理 |

**一键建议操作** 只处理建议为禁用、启用、删除的账号。  
**批量禁用 / 批量删除** 会按当前筛选结果强制执行，请确认筛选条件后再操作。

## 数据说明

- 巡检结果会保存在 CPA 工作目录下的 `data/grok-inspection/results.json`
- 结果文件只保存展示所需信息，不保存完整 token
- 实时自动禁用默认开启：命中 free-usage-exhausted / permission-denied / 401 时会自动禁用；可在「实时自动禁用」页关闭。巡检页的建议操作仍需确认后执行
- 删除操作会删除对应 Auth 凭证，恢复需要重新登录

## License

MIT

## 社区

本开源项目与 LINUX DO 社区相关联，并致谢该社区。
