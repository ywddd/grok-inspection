# Architecture

Grok Inspection is a native CPA plugin loaded as a C shared library. It registers a Management UI and several Management API routes, then runs Grok account inspection and account actions inside the CPA process.

## Runtime Layout

```text
Browser UI
  |
  | CPA Management Key
  v
CPA Management API
  |
  | management.handle
  v
grok-inspection plugin
  |
  +-- host.auth.list/get/save   read and update CPA auth files
  +-- host.http.do              probe upstream Grok with the account token
  +-- local management HTTP     delete CPA auth files
  +-- results.json              persist latest inspection snapshot
```

The browser never calls Grok directly. All probing is performed by the plugin through CPA host callbacks.

## Plugin Registration

`main.go` handles the CPA plugin ABI methods:

| ABI method | Purpose |
|------------|---------|
| `plugin.register` / `plugin.reconfigure` | Return plugin metadata and capabilities |
| `management.register` | Register the Management routes and menu resource |
| `management.handle` | Serve the UI and handle status/start/stop/apply/action requests |
| shutdown | Stop background work during plugin unload |

The Management menu resource is `/v0/resource/plugins/grok-inspection/status`.

The Management API base is `/v0/management/plugins/grok-inspection`.

## Management Routes

| Method | Route | Purpose |
|--------|-------|---------|
| `GET` | `/status` | Return progress, summary, results, and recent row action reports |
| `POST` | `/start` | Start full or incremental inspection |
| `POST` | `/stop` | Stop scheduling new inspection work |
| `POST` | `/apply` | Apply recommended or forced bulk actions asynchronously |
| `POST` | `/action` | Run one row action asynchronously |

`/status` supports light polling with `include_results=0` or `light=1`. Light status omits the full result list and is used while inspection or actions are running.

## Inspection Flow

1. UI posts `/start` with worker count and disabled-account filters.
2. The engine lists CPA auth entries with `host.auth.list`.
3. Entries are filtered to Grok/xAI accounts.
4. Each selected account is fetched with `host.auth.get`.
5. The engine extracts a token and probes Grok through `host.http.do`.
6. The response is classified into a result and recommended action.
7. Results are appended in memory and periodically persisted.

Full inspection clears previous results. Incremental inspection keeps existing results and only probes accounts that are not already represented by a stable identity such as `auth_index` or file metadata.

## Grok Probe

The probe selects a model from:

```text
GET https://cli-chat-proxy.grok.com/v1/models
```

Then it tests the account with:

```text
POST https://cli-chat-proxy.grok.com/v1/responses
```

If the main probe receives common auth, quota, or permission errors, it falls back to:

```text
POST https://cli-chat-proxy.grok.com/v1/chat/completions
```

The probe classifies HTTP status and structured error fields. It does not rely on natural-language model output.

## Classification

| Classification | Default action | Main signal |
|----------------|----------------|-------------|
| `healthy` | `keep`, or `enable` if currently disabled | Probe returned 2xx |
| `permission_denied` | `disable`, or `keep` if already disabled | 402/403 or permission/banned/suspended text |
| `quota_exhausted` | `disable`, or `keep` if already disabled | 429 or quota/limit exhausted text |
| `reauth` | `delete` | 401 or expired/invalid token text |
| `model_unavailable` | `keep` | 404 or model unavailable text |
| `probe_error` | `keep` | Request, decode, or unexpected probe failure |
| `unknown` | `keep` | Fallback when no reliable signal exists |

## Account Actions

Bulk actions use `/apply`; single-row actions use `/action`.

Disable and enable are applied by reading the auth JSON with `host.auth.get`, changing its `disabled` field, and writing it back with `host.auth.save`.

Delete uses CPA Management HTTP against the local CPA process. The plugin reuses the page Management Key from request headers when available, with `MANAGEMENT_PASSWORD` or `CPA_MANAGEMENT_KEY` as fallback for headless container setups.

Both bulk and row actions are asynchronous:

- `/apply` returns `202 Accepted` and progress is read from `/status`.
- `/action` returns `action_seq`; the UI polls light `/status` until `recent_row_actions` reports that sequence.

## Persistence

The latest snapshot is stored as compact JSON:

```text
data/grok-inspection/results.json
```

Set `GROK_INSPECTION_DATA_DIR` to override the directory.

The persisted file contains display-oriented result data only. It does not store complete auth JSON or tokens.

## Concurrency

Inspection worker count is user-configurable and validated by the engine. Bulk operations also run asynchronously with bounded concurrency for enable/disable and batched deletes.

The engine keeps all mutable state behind a mutex and exposes snapshots to the UI. Status requests must stay cheap and must not trigger host calls or upstream probes.
