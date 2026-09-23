# Verdent

A clean-room Verdent API compatibility gateway focused on reliable agent/tool workflows, multi-account routing, and stable per-account egress.

## Current development status

Implemented on `fix/live-protocol-alignment`:

- OpenAI-compatible `POST /v1/chat/completions`
- OpenAI Responses-compatible `POST /v1/responses`
- dynamic `GET /v1/models`
- streaming text, reasoning, native function calls, and Codex-style `custom apply_patch`
- AES-GCM Verdent protocol codec and request envelope
- multi-account routing with session affinity, cooldown, suspension, and manual disable states
- one isolated HTTP/HTTPS outbound proxy per account
- persistent managed account store
- embedded responsive management console
- admin login sessions with HttpOnly cookies
- three account enrollment paths: browser PKCE, Verdent Desktop import, and manual token import

The browser PKCE path now follows the currently observed Verdent shape: `/auth` with S256 `challenge` + `state`, callback `code` exchange at `/passport/pkce/callback`, persisted expiry/refresh credentials, and automatic refresh through `/passport/token/refresh` when available. It is still marked **experimental until one live production login is verified on the deployed gateway**.

## Management console

The console is embedded in the Go binary; there is no Node runtime dependency.

Default address:

```text
http://127.0.0.1:5084/
```

The UI follows the same lightweight Cloudflare-style interaction pattern as the related OpenCode Proxy console: desktop sidebar, responsive mobile drawer, KPI cards, tables, modals, toast feedback, and light/dark themes.

Configure at minimum:

```bash
VERDENT_ADMIN_USER=admin
VERDENT_ADMIN_PASSWORD=change-admin-password
VERDENT_API_KEY=change-api-key
```

If `VERDENT_ADMIN_PASSWORD` is omitted, the console falls back to `VERDENT_API_KEY` as its login password.

### Account enrollment

The Accounts page provides:

1. **Browser sign-in (experimental until live-verified)** — creates a PKCE verifier/challenge, opens the Verdent authorization page, validates OAuth `state`, exchanges `code + codeVerifier`, persists access/refresh credentials and refreshes expiring access tokens when the service returns a refresh token.
2. **Import from Verdent Desktop** — best-effort import from the current local desktop credential. macOS Keychain and Linux Secret Service have direct helpers; automatic Windows Credential Manager import is not enabled yet.
3. **Manual token** — useful for debugging, headless servers, or fallback migration.

Re-authorizing an existing account updates its token while preserving its existing Device ID and bound proxy.

Managed accounts default to:

```text
data/accounts.json
```

The file is written with restrictive permissions and is excluded from Git. Tokens and proxy credentials are not returned by management APIs; proxy URLs are masked in the UI.

## Per-account proxy affinity

Each account owns an isolated `http.Client` / `http.Transport`. A proxy configured for one account is therefore not a process-global proxy and cannot silently rotate to another account's egress.

Supported proxy schemes:

```text
http://
https://
```

SOCKS is intentionally rejected until a dedicated transport is implemented.

Optional request routing headers:

- `X-Verdent-Account: <account-id>` — explicitly select one eligible account.
- `X-Verdent-Session-ID: <stable-session-id>` — preserve session affinity to one eligible account.

## Architecture

```text
OpenCode / Hermes / Codex / OpenAI-compatible clients
                    |
                    v
          API compatibility layer
     Chat Completions / Responses / Models
                    |
                    v
          Canonical message/event model
                    |
                    v
             Verdent protocol core
       native tools / SSE / AES-GCM envelope
                    |
                    v
       Account router + account state machine
                    |
                    v
          Per-account HTTP transport
                    |
                    v
           Verdent cloud upstream
```

## Development run

1. Export the configuration from `.env.example`.
2. Supply the Verdent protocol version/beta/sign values verified for the current client.
3. Set an administrator password and downstream API key.
4. Run:

```bash
go run ./cmd/verdent
```

You can start with **zero accounts**, log in to the management console, and add accounts there.

When running behind a reverse proxy, configure the browser-visible origin so OAuth callbacks point back to the correct gateway:

```bash
VERDENT_PUBLIC_BASE_URL=https://verdent.example.com
```

Client base URL:

```text
http://127.0.0.1:5084/v1
```


## Release binaries

The repository includes a dedicated GitHub Actions release workflow:

`.github/workflows/build-windows.yml`

It currently builds standalone binaries for:

```text
Windows x64  -> GOOS=windows GOARCH=amd64 CGO_ENABLED=0
Ubuntu/Linux x64 -> GOOS=linux GOARCH=amd64 CGO_ENABLED=0
```

### Manual build artifacts

Open **Actions → build-release → Run workflow**.

The workflow produces two artifacts:

```text
verdent-windows-amd64
verdent-linux-amd64
```

Windows assets:

```text
verdent.exe
verdent.exe.sha256
verdent-windows-amd64.zip
verdent-windows-amd64.zip.sha256
```

Ubuntu/Linux assets:

```text
verdent
verdent.sha256
verdent-linux-amd64.tar.gz
verdent-linux-amd64.tar.gz.sha256
```

Both packages also include `.env.example` and `README.md`.

### Release build

Push a version tag such as:

```bash
git tag v0.1.0-alpha.2
git push origin v0.1.0-alpha.2
```

The workflow runs tests, builds both platforms, creates SHA256 checksums, and uploads the binaries and archives to the matching GitHub Release.

Normal branch CI also cross-builds both Windows x64 and Ubuntu/Linux x64 binaries after `go test ./...` and `go vet ./...`, so platform build regressions are caught before release.


## Current Desktop-alignment behavior

The current protocol path incorporates the September 22 findings from an independent Verdent2API implementation and keeps the evidence-backed pieces isolated behind configuration:

- a captured Desktop `system` ciphertext can be loaded from `VERDENT_SYSTEM_TEMPLATE_FILE`; downstream client system instructions are then folded into the first user message instead of replacing the fingerprinted upstream field
- message content is emitted as block arrays with a leading `<timestamp>` block and `cache_control: {"type":"ephemeral"}` on the final block
- only assistant history carries the upstream `model` field
- `native_api` defaults to the captured value (`false`) and can be overridden explicitly
- each account has its own upstream pacing gate (default 1.2 s between starts) plus bounded retry/backoff for 429 and explicit retryable 5xx responses
- retries always remain on that account's isolated transport/proxy; there is no fallback to an environment/system proxy

## Design principles

1. **Protocol first** — Verdent transport stays isolated from API compatibility code.
2. **Native tools first** — structured tools are the primary path; prompt-emulated tools are not the normal path.
3. **Dynamic catalog** — model capabilities are discovered instead of maintained as a static model table.
4. **Account state machine** — auth failure, quota/rate limiting, suspension, and manual disable are distinct states.
5. **Stable egress** — each account owns its outbound transport.
6. **Secret boundary** — access tokens and proxy credentials stay server-side.
7. **Clean-room implementation** — compatible public implementations may be referenced under their licenses, while private credentials/captures are not stored in the repository.

## Verification boundary

Synthetic protocol, compatibility, account-routing, Responses, UI-management, and end-to-end HTTP tests are included in CI.

The project is **not yet declared compatible with the latest Verdent Desktop build**. Before merging this development work as a release candidate, the current production client should be used to verify:

- current `X-Version-Code`, beta header, and proxy-sign behavior
- browser PKCE enrollment and refresh behavior against the current authentication service
- captured Desktop `system` fingerprint / model catalog version
- current model catalog response
- native tool-call streaming, especially `apply_patch`
- account suspension / quota behavior

See `docs/ARCHITECTURE.md`, `docs/PROTOCOL.md`, `CONTEXT.md`, and `THIRD_PARTY_NOTICES.md` for engineering notes.
