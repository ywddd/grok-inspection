# Grok Inspection

CPA（CLIProxyAPI）原生插件，用于在后台巡检 xAI/Grok 账号状态，并给出可执行的处理建议。

版本：`0.1.9` · 菜单名：**Grok 账号巡检**

## 功能

- 完整巡检、增量巡检、按分类重巡 Grok/xAI 账号
- 识别健康、权限被拒、额度用尽、需重新登录、模型不可用、探测异常
- 后台执行巡检和批量操作，切换页面不丢任务
- 支持一键执行建议、批量禁用、批量删除、单账号处理
- 巡检结果落盘，页面重新打开后可恢复
- 支持导出当前筛选结果为 JSON/TXT

## 安装

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

## Docker

CPA 跑在 Docker 中时，需要把插件拷进容器内的插件目录，并重启容器：

```bash
docker ps
docker cp ./grok-inspection.so cpa:/app/plugins/linux/amd64/grok-inspection.so
docker restart cpa
docker logs -f cpa
```

容器名和插件目录以你的 CPA 镜像实际路径为准。删除账号功能需要 CPA 进程能读取 `MANAGEMENT_PASSWORD` 或 `CPA_MANAGEMENT_KEY`；网页填写的 Management Key 也会被插件复用于删除请求。

## 使用

1. 打开 **Grok 账号巡检**。
2. 输入 CPA Management Key。
3. 选择并发数、是否包含已禁用账号、是否仅巡检已禁用账号。
4. 点击 **开始巡检** 或 **增量巡检**。
5. 根据结果执行 **一键建议操作**、批量禁用、批量删除或单账号操作。

巡检和批量操作都在插件后台异步执行。页面关闭或切换后任务仍会继续，重新打开页面会读取当前进度和上次结果。

## 结果说明

| 结果 | 默认建议 | 含义 |
|------|----------|------|
| 健康 | 保留；如果已禁用则建议启用 | 对话探测成功，账号可用 |
| 权限被拒 | 禁用 | 账号没有 chat endpoint 权限、被拒绝或疑似权限受限 |
| 额度用尽 | 禁用 | 免费额度或使用额度耗尽，暂时不适合继续调度 |
| 需重新登录 | 删除 | token 失效或登录态不可用，删除后需要在 CPA 重新登录 |
| 模型不可用 | 保留 | 当前探测模型不可用，不一定代表账号失效 |
| 探测异常 | 保留 | 网络、上游响应、字段缺失等异常，建议复查后再处理 |

**执行建议操作** 只处理建议为禁用、启用、删除的账号。  
**批量禁用/删除** 会按当前筛选结果强制执行，请确认筛选条件后再操作。

## 构建

需要 Go 1.21+ 和可用的 C 编译器：

```bash
# Linux/macOS
./build.sh

# Windows PowerShell
./build.ps1
```

产物会输出到 `dist/`。

发布版本使用 tag 触发 GitHub Actions：

```bash
git tag v0.1.9
git push origin v0.1.9
```

## 数据与安全

- 巡检结果保存在 `data/grok-inspection/results.json`，或 `GROK_INSPECTION_DATA_DIR/results.json`
- 结果文件只保存展示所需信息，不保存完整 token/Auth JSON
- 插件不会自动禁用或删除账号，必须由用户点击确认
- 删除操作会删除 CPA Auth 凭证文件，恢复需要重新登录
- 不要把 Management Key、Auth JSON 或包含隐私的巡检结果提交到公开仓库

## 文档

- [架构设计](docs/ARCHITECTURE.md)

## License

MIT

## 社区
本开源项目已链接并认可 LINUX DO 社区。
