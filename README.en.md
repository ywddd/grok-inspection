# Grok Inspection

> [中文](README.md) | **English**

CPA (CLIProxyAPI) plugin for bulk xAI/Grok account health checks, with suggested disable / enable / delete actions.

Version: `0.1.13` · Menu: **Grok Account Inspection**

## Features

- Full, incremental, and classification-scoped inspection for Grok/xAI accounts
- **Dual-gateway probe**: CLI chat-proxy first; if CLI denies, probe `api.x.ai` with the **same token** (**no re-login**)
- Detects healthy, **API gateway OK** (CLI denied but official API works), permission denied, free-usage exhausted, reauth required, model unavailable, and probe errors
- One-click write of `using_api=true` + `base_url=https://api.x.ai/v1` (token unchanged; needs CPA `using_api` semantics)
- Upload accounts file (`email----password----sso`) and silently refresh tokens for matching reauth/403 accounts
- Background inspection and bulk actions continue if you leave the page
- One-click suggested actions, bulk disable/delete, and single-account actions
- Results are persisted and restored when you reopen the page
- Export filtered results as JSON/TXT

## Install

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

## Docker

If CPA runs in Docker, copy the plugin into the container plugin directory and restart. Use your real container name and plugin path:

```bash
docker cp ./grok-inspection.so <container>:<plugin-dir>/grok-inspection.so
docker restart <container>
```

Disable and delete actions use the CPA Management Key entered on the page.

The plugin calls the CPA Management API through loopback by default. For Docker, port-mapped, or custom-listen-port deployments where the plugin cannot reach the actual management port, set this explicitly on the CPA process:

```bash
CPA_MANAGEMENT_BASE_URL=http://127.0.0.1:<actual-port>
```

Use `https://` when TLS is enabled. With an explicit value, a failed request will not fall back to the browser request Origin.

## Usage

1. Open **Grok Account Inspection**.
2. Enter the CPA Management Key.
3. Choose concurrency and disabled-account filters.
4. Click **Start Inspection** or **Incremental Inspection**.
5. If you see **API available**: CLI denied but `api.x.ai` works — click **Apply api.x.ai base_url** (`using_api` + `base_url`, no re-login).
6. Apply **suggested actions**, bulk disable/delete, or single-account actions.
7. For reauth: upload the accounts file first, then **Re-login matching accounts**.

Inspection and bulk actions run in the background. Closing or switching pages does not cancel them; reopen the page to see progress and the last results.

**Stop** ends inspection / reauth / base_url apply. Accounts not yet probed are marked as stopped / not probed.

### Older CPA fallback (openai-compatibility)

If your CPA build does not yet route native `grok-4.5` via `using_api`, add an openai-compatibility entry (`base-url: https://api.x.ai/v1`, access_token as api-key, model alias e.g. `grok-4.5-apixai`). Never put tokens in results or browser responses. Prefer the native path after upgrading CPA.

## Results

| Result | Default suggestion | Meaning |
|--------|--------------------|---------|
| Healthy | Keep; enable if currently disabled | CLI chat probe succeeded |
| API available | Switch to api.x.ai | CLI denied but official API works (same token, no re-login) |
| Permission denied | Disable | Chat permission denied (and official API also fails / not usable) |
| Free-usage exhausted | Disable | Free usage is exhausted for now |
| Reauth required | Delete | Login/token is invalid; re-login in CPA after delete |
| Model unavailable | Keep | Probe model unavailable; account may still be fine |
| Probe error | Keep | Network/upstream issue; recheck before acting |

**Suggested actions** only cover disable / enable / delete recommendations.  
**Bulk disable / delete** force-applies to the current filter; confirm the filter first.

## Data

- Results are stored at `data/grok-inspection/results.json` under the CPA working directory
- Result files store display fields only, not full tokens
- The plugin never auto-disables or auto-deletes accounts; confirmation is required
- Delete removes the CPA auth credential; recovery requires re-login

## License

MIT

## Community

This open-source project is linked with and acknowledges the LINUX DO community.
