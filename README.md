# Verdent

A clean-room Verdent API compatibility gateway focused on reliable agent/tool workflows.

## Implemented on the development branch

- OpenAI-compatible `POST /v1/chat/completions`
- OpenAI-compatible `GET /v1/models` backed by dynamic Verdent model discovery
- streaming text, reasoning and native structured tool-call translation
- non-streaming tool-call aggregation
- AES-GCM Verdent protocol codec and request envelope
- multi-account routing with session affinity, cooldown and suspension states
- per-account HTTP/HTTPS outbound proxy affinity
- optional local Bearer API authentication

Planned next: OpenAI Responses API, Anthropic Messages API, persistent account management, live current-client verification, and integration regression tests for OpenCode/Hermes/Codex.

## Architecture

```text
OpenAI / Anthropic clients
          |
          v
API compatibility layer
          |
          v
Canonical message/event model
          |
          v
Verdent protocol core
          |
          v
Account router + per-account transport
          |
          v
Verdent upstream
```

## Development run

1. Copy `.env.example` values into your process environment.
2. Configure either a single account with `VERDENT_TOKEN` + `VERDENT_DEVICE_ID`, or a multi-account JSON file with `VERDENT_ACCOUNTS_FILE`.
3. Supply the Verdent protocol version/beta/sign values verified for the current client.
4. Run:

```bash
go run ./cmd/verdent
```

Default listen address: `:5084`.

Example client base URL:

```text
http://127.0.0.1:5084/v1
```

Optional account/session routing headers:

- `X-Verdent-Account: <account-id>` selects one eligible account explicitly.
- `X-Verdent-Session-ID: <stable-session-id>` keeps subsequent requests on the same eligible account.

## Design principles

1. **Protocol first** — Verdent transport stays isolated from client API compatibility.
2. **Native tools first** — structured `tools` / `tool_choice` are the primary path; prompt-based tool emulation is not the normal path.
3. **Dynamic catalog** — model capabilities are discovered at runtime rather than hard-coded.
4. **Account state machine** — auth failure, throttling/quota and account suspension are distinct conditions.
5. **Stable egress** — each account owns an isolated HTTP transport and can be pinned to one proxy.
6. **Clean-room implementation** — source is not copied from projects without compatible licensing.

## Status / verification boundary

The implementation is covered by synthetic protocol and end-to-end tests. It is **not yet declared compatible with the latest Verdent desktop build** until the current client request headers, envelope fields, tool stream and catalog behavior are verified against a sanitized live capture.

See `docs/ARCHITECTURE.md`, `docs/PROTOCOL.md`, `CONTEXT.md`, and `THIRD_PARTY_NOTICES.md` for engineering notes.
