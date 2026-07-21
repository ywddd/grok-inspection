# Grok Inspection

> [中文](README.md) | **English**

CPA (CLIProxyAPI) plugin for bulk xAI/Grok account health checks, with suggested disable / enable / delete actions.

## Note

This is a **pure vibe-coding** plugin: it works and is practical, but the code may not be polished to traditional engineering standards.

- **Issues and PRs are welcome** — bug fixes, features, UX, and refactors.
- **If you prefer not to rely on vibe-coded plugins**, use **CPA Manager Plus** (or a similar management panel) for account inspection / ops instead.
- This plugin is a lightweight, **optional** Grok/xAI inspection add-on — not an official reference implementation.

Version: `0.1.13` · Menu: **Grok Account Inspection**

## Features

- Full, incremental, and classification-scoped inspection for Grok/xAI accounts
- Detects healthy, permission denied, free-usage exhausted, reauth required, model unavailable, and probe errors
- Background inspection and bulk actions continue if you leave the page
- One-click suggested actions, bulk disable/delete, and single-account actions
- Results are persisted and restored when you reopen the page
- Export filtered results as JSON/TXT
- Real-time autoban (on by default): free-usage cools down after 24h; 403/401 need manual unban

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
5. Apply **suggested actions**, bulk disable/delete, or single-account actions.

Inspection and bulk actions run in the background. Closing or switching pages does not cancel them; reopen the page to see progress and the last results.

**Stop** ends the current run immediately. Accounts not yet probed are marked as stopped / not probed.

## Results

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

## Data

- Results are stored at `data/grok-inspection/results.json` under the CPA working directory
- Result files store display fields only, not full tokens
- Real-time autoban is on by default: free-usage-exhausted / permission-denied / 401 are auto-disabled (toggle off on the Autoban page). Inspection suggested actions still require confirmation
- Delete removes the CPA auth credential; recovery requires re-login

## License

MIT

## Community

This open-source project is linked with and acknowledges the LINUX DO community.
