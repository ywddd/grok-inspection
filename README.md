# Grok Inspection

`grok-inspection` is a CPA Management API plugin for inspecting xAI/Grok
credential health in the background.

## Features

- Runs inspections inside the CPA plugin process, so jobs continue when the
  management page is changed or closed.
- Probes the official Grok CLI chat proxy with the same identity headers used
  by CPA's xAI executor.
- Tries the Responses API first and falls back to Chat Completions.
- Classifies healthy, permission-denied, quota-exhausted, expired-login,
  unavailable-model, and probe-error states.
- Recommends disabling credentials with denied chat access or exhausted quota.
- Recommends re-enabling disabled credentials that pass the chat probe.
- Supports configurable concurrency, disabled-account filters, pagination,
  individual actions, and bulk application of recommendations.

## Requirements

- CLIProxyAPI/CPA with native plugin support
- Go 1.21 or newer
- A C compiler for `-buildmode=c-shared`

## Build

Linux/macOS:

```bash
./build.sh
```

Windows PowerShell:

```powershell
./build.ps1
```

The binary is written to `dist/`.

## Release Assets

Official CPA plugin distribution reads the latest GitHub Release. Tag releases
with a dotted version such as `v0.1.0`. The release workflow publishes:

```text
grok-inspection_0.1.0_linux_amd64.zip
checksums.txt
```

The zip contains `grok-inspection.so` at its root.

## Install

Copy the Linux artifact to the CPA plugin directory:

```text
plugins/linux/amd64/grok-inspection.so
```

Enable it in `config.yaml`:

```yaml
plugins:
  enabled: true
  configs:
    grok-inspection:
      enabled: true
      priority: 1
```

Restart CPA and open:

```text
/v0/resource/plugins/grok-inspection/status
```

The resource page asks for the CPA Management Key before calling authenticated
Management API routes. The key is stored only in browser local storage.

## Management API

All routes below require CPA Management API authentication:

- `GET /v0/management/plugins/grok-inspection/status`
- `POST /v0/management/plugins/grok-inspection/start`
- `POST /v0/management/plugins/grok-inspection/stop`
- `POST /v0/management/plugins/grok-inspection/apply`
- `POST /v0/management/plugins/grok-inspection/action`

## Safety

- The plugin does not automatically apply recommendations after an inspection.
- Bulk and individual enable/disable actions must be explicitly requested.
- `permission_denied` and quota exhaustion are recommendations, not proof that
  the physical credential file should be deleted.
- Never commit CPA configuration, Management Keys, auth JSON files, or logs.

## License

MIT
