# Grok Inspection

CPA (CLIProxyAPI) plugin for bulk xAI/Grok account health checks, with suggested disable / enable / delete actions.

Version: `0.1.9` · Menu: **Grok 账号巡检** / **Grok Account Inspection**

---

## 中文

### 功能

- 完整巡检、增量巡检、按分类重巡 Grok/xAI 账号
- 识别健康、权限被拒、额度用尽、需重新登录、模型不可用、探测异常
- 后台执行巡检和批量操作，切换页面不丢任务
- 支持一键执行建议、批量禁用、批量删除、单账号处理
- 巡检结果落盘，页面重新打开后可恢复
- 支持导出当前筛选结果为 JSON/TXT

### 安装

从 [Releases](https://github.com/ywddd/grok-inspection/releases) 下载与你的 CPA 平台匹配的压缩包：

| 平台 | 文件 |
|------|------|
| Linux amd64 | `grok-inspection_*_linux_amd64.zip` |
| Linux arm64 | `grok-inspection_*_linux_arm64.zip` |
| Windows amd64 | `grok-inspection_*_windows_amd64.zip` |
| macOS arm64 | `grok-inspection_*_darwin_arm64.zip` |

解压后把插件文件放到 CPA 插件目录，常见文件名如下：

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

### Docker

如果 CPA 运行在 Docker 中，把插件拷到容器内的插件目录后重启容器。容器名和插件路径以你的实际环境为准，例如：

```bash
docker cp ./grok-inspection.so <容器名>:<插件目录>/grok-inspection.so
docker restart <容器名>
```

删除 / 禁用账号时，会使用页面里填写的 CPA Management Key。

### 使用

1. 打开 **Grok 账号巡检**。
2. 输入 CPA Management Key。
3. 选择并发数、是否包含已禁用账号、是否仅巡检已禁用账号。
4. 点击 **开始巡检** 或 **增量巡检**。
5. 根据结果执行 **一键建议操作**、批量禁用、批量删除或单账号操作。

巡检和批量操作都在后台执行。页面关闭或切换后任务仍会继续；重新打开页面可查看当前进度和上次结果。

点 **停止** 会立即结束本轮巡检：尚未探测的账号会标记为「已停止，未探测」。

### 结果说明

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

### 数据说明

- 巡检结果会保存在 CPA 工作目录下的 `data/grok-inspection/results.json`
- 结果文件只保存展示所需信息，不保存完整 token
- 插件不会自动禁用或删除账号，需你在页面确认后执行
- 删除操作会删除对应 Auth 凭证，恢复需要重新登录

---

## English

### Features

- Full, incremental, and classification-scoped inspection for Grok/xAI accounts
- Detects healthy, permission denied, free-usage exhausted, reauth required, model unavailable, and probe errors
- Background inspection and bulk actions continue if you leave the page
- One-click suggested actions, bulk disable/delete, and single-account actions
- Results are persisted and restored when you reopen the page
- Export filtered results as JSON/TXT

### Install

Download the package for your CPA platform from [Releases](https://github.com/ywddd/grok-inspection/releases):

| Platform | File |
|----------|------|
| Linux amd64 | `grok-inspection_*_linux_amd64.zip` |
| Linux arm64 | `grok-inspection_*_linux_arm64.zip` |
| Windows amd64 | `grok-inspection_*_windows_amd64.zip` |
| macOS arm64 | `grok-inspection_*_darwin_arm64.zip` |

Extract the plugin binary into your CPA plugins directory:

```text
grok-inspection.so      # Linux
grok-inspection.dll     # Windows
grok-inspection.dylib   # macOS
```

Enable it in CPA config:

```yaml
plugins:
  enabled: true
  configs:
    grok-inspection:
      enabled: true
      priority: 1
```

Restart CPA, open **Grok Account Inspection**, and enter the CPA Management Key.

### Docker

If CPA runs in Docker, copy the plugin into the container plugin directory and restart. Use your real container name and plugin path:

```bash
docker cp ./grok-inspection.so <container>:<plugin-dir>/grok-inspection.so
docker restart <container>
```

Disable and delete actions use the CPA Management Key entered on the page.

### Usage

1. Open **Grok Account Inspection**.
2. Enter the CPA Management Key.
3. Choose concurrency and disabled-account filters.
4. Click **Start Inspection** or **Incremental Inspection**.
5. Apply **suggested actions**, bulk disable/delete, or single-account actions.

Inspection and bulk actions run in the background. Closing or switching pages does not cancel them; reopen the page to see progress and the last results.

**Stop** ends the current run immediately. Accounts not yet probed are marked as stopped / not probed.

### Results

| Result | Default suggestion | Meaning |
|--------|--------------------|---------|
| Healthy | Keep; enable if currently disabled | Chat probe succeeded |
| Permission denied | Disable | Chat permission denied or account restricted |
| Free-usage exhausted | Disable | Free usage is exhausted for now |
| Reauth required | Delete | Login/token is invalid; re-login in CPA after delete |
| Model unavailable | Keep | Probe model unavailable; account may still be fine |
| Probe error | Keep | Network/upstream issue; recheck before acting |

**Suggested actions** only cover disable / enable / delete recommendations.  
**Bulk disable / delete** force-applies to the current filter; confirm the filter first.

### Data

- Results are stored at `data/grok-inspection/results.json` under the CPA working directory
- Result files store display fields only, not full tokens
- The plugin never auto-disables or auto-deletes accounts; confirmation is required
- Delete removes the CPA auth credential; recovery requires re-login

---

## License

MIT

## Community

This open-source project is linked with and acknowledges the LINUX DO community.