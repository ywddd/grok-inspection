# Grok Inspection

> [中文](README.md) | **English**

CPA (CLIProxyAPI) plugin for bulk xAI/Grok account health checks, with suggested disable / enable / delete actions.

Version: `0.2.1` · Menu: **Grok Account Inspection**

## Features

- Full, incremental, and classification-scoped inspection for Grok/xAI accounts
- Detects healthy, permission denied, free-usage exhausted, reauth required, model unavailable, and probe errors
- Background inspection and bulk actions continue if you leave the page
- One-click suggested actions, bulk disable/delete, and single-account actions
- Results are persisted and restored when you reopen the page
- Export filtered results as JSON/TXT
- Optional cron auto-inspection (off by default); timed path can auto-delete permission-denied, disable quota-exhausted, and enable healthy-but-disabled accounts

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

## Usage

1. Open **Grok Account Inspection**.
2. Enter the CPA Management Key.
3. Choose concurrency and disabled-account filters.
4. Click **Start Inspection** or **Incremental Inspection**.
5. Apply **suggested actions**, bulk disable/delete, or single-account actions.

Inspection and bulk actions run in the background. Closing or switching pages does not cancel them; reopen the page to see progress and the last results.

**Stop** ends the current run immediately. Accounts not yet probed are marked as stopped / not probed.

## Auto scheduled inspection

The **Auto scheduled inspection** panel at the bottom of the page:

| Setting | Meaning |
|---------|---------|
| Enable | Off by default; when on, fires on cron in the process local timezone |
| Cron | 5-field expression; default `0 3 * * *` (03:00 daily) |
| Workers | Workers for timed full inspection (1–16, default 6) |
| Auto-delete permission denied | After a timed full run only, delete this run's `permission_denied` |
| Auto-disable quota exhausted | After a timed full run only, disable this run's non-disabled `quota_exhausted` |
| Auto-enable healthy disabled | After a timed full run only, enable this run's disabled `healthy` |

Rules:

- Timed runs are always **full** inspections and **include disabled** accounts.
- Auto actions run **only** on the timed path; manual full/incremental/classify runs never auto-mutate accounts.
- Auto delete/disable/enable need process env `MANAGEMENT_PASSWORD` or `CPA_MANAGEMENT_KEY` (page key is not persisted for this).
- If the engine is busy at fire time, the tick is **skipped**.
- Config is stored at `data/grok-inspection/schedule.json`.

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
- Schedule config is stored at `schedule.json` in the same directory
- Result files store display fields only, not full tokens
- Auto actions are off by default; when enabled, they run only after a **timed** inspection finishes
- Manual disable/delete still requires explicit confirmation on the page
- Delete removes the CPA auth credential; recovery requires re-login

## License

MIT

## Community

This open-source project is linked with and acknowledges the LINUX DO community.
