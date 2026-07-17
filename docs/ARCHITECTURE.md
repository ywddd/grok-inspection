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
| `POST` | `/stop` | Stop inspection / reauth / base_url apply jobs |
| `POST` | `/apply` | Apply recommended or forced bulk actions asynchronously |
| `POST` | `/action` | Run one row action asynchronously |
| `POST` | `/credentials` | Upload accounts file (email----password----sso); secrets in memory only |
| `POST` | `/credentials/clear` | Clear uploaded credentials from memory |
| `POST` | `/reauth/start` | Silent token refresh for matched reauth/403 accounts |
| `POST` | `/baseurl/apply` | Write `using_api=true` + `base_url=https://api.x.ai/v1` for CLI-denied / API-ok accounts |

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

Each inspection run uses a fixed free-tier probe model (no per-account `/v1/models`):

```text
model = grok-4.5  # free accounts are remapped upstream to grok-4.5-build-free
```

Each host HTTP call has a 25s timeout with one timeout-only retry (short backoff). A whole account probe hard-caps at ~90s so one hung upstream cannot stall the job forever (CLI + optional API probe).

Every account is tested first on the CLI chat-proxy:

```text
POST https://cli-chat-proxy.grok.com/v1/responses
```

Fallback on CLI is used only when the primary result is **ambiguous** (temporary 429, 5xx, unknown, model unavailable, etc.):

```text
POST https://cli-chat-proxy.grok.com/v1/chat/completions
```

Definitive primary results skip CLI fallback: `healthy`, `quota_exhausted` (free-usage only), `permission_denied`, `reauth`. When both are tried, free-usage / permission / reauth from primary remain authoritative if fallback returns success.

### Dual gateway (CLI + official API)

When CLI classification is `permission_denied` (or ambiguous 402/403), the same access_token is probed on the official API **without re-login** and **without CLI identity headers**:

```text
POST https://api.x.ai/v1/chat/completions
```

| CLI | api.x.ai | Result |
|-----|----------|--------|
| healthy | (not needed) | `healthy`, preferred `cli-chat-proxy` |
| permission_denied | 2xx | **`api_gateway_ok`**, action `switch_base_url` (token works; no re-login) |
| permission_denied | fail | stays `permission_denied` (+ gateway note) |
| reauth / quota | (skipped) | never overridden by API probe |

Result fields: `cli_http_status`, `api_http_status`, `preferred_base_url`, `auth_base_url`, `using_api`, `gateway_note`.

### One-click base_url apply

`POST /baseurl/apply` writes on selected auth files (via `host.auth.save`):

- `using_api: true`
- `base_url: https://api.x.ai/v1`

Tokens are unchanged. CPA chat then honors the file when `using_api` is true (see CPA `xaiChatBaseURL`). Optional post-write re-probe of api.x.ai.

### openai-compatibility fallback (older CPA)

If production CPA is older and does not yet route native `grok-4.5` via `using_api`, add a config entry under `openai-compatibility` (token as api-key, model alias e.g. `grok-4.5-apixai`, `base-url: https://api.x.ai/v1`). Do not put tokens in results.json or the browser UI. After a CPA build that honors `using_api`, native `grok-4.5` is preferred.

The probe classifies HTTP status and structured error fields. It does not rely on natural-language model output.

## Classification

| Classification | Default action | Main signal |
|----------------|----------------|-------------|
| `healthy` | `keep`, or `enable` if currently disabled | CLI probe returned 2xx |
| `api_gateway_ok` | `switch_base_url` (until applied → `keep`) | CLI denied; official api.x.ai 2xx with same token |
| `permission_denied` | `disable`, or `keep` if already disabled | 402/403 or permission/banned/suspended text (and API also fails or not probed) |
| `quota_exhausted` | `disable`, or `keep` if already disabled | Only Grok free-usage body/code (`subscription:free-usage-exhausted`, `free-usage-exhausted`, included free usage exhausted). Bare HTTP 429 is **not** quota |
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

The engine keeps all mutable state behind a mutex and exposes snapshots to the UI. Status requests must stay cheap and must not trigger host calls or upstream probes. Result snapshots are copied under the lock and written to disk outside the critical section (async mid-run, synchronous on finish).

Stop is cooperative: no new accounts are scheduled; in-flight probes still complete and are written; unscheduled accounts are recorded as cancelled (已停止，未探测) so progress can reach total.

Source layout: `engine.go` (job lifecycle), `probe.go` (dual-gateway HTTP probe), `baseurl.go` (using_api + base_url apply), `reauth.go` (silent OAuth refresh), `identity.go` (account matching), `apply.go` (bulk/row actions), `management.go` (CPA management HTTP), `ui.go` (embedded Management page).
