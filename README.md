# Verdent

A clean-room Verdent API compatibility gateway focused on reliable agent/tool workflows.

## Goals

- OpenAI-compatible `/v1/chat/completions`
- OpenAI Responses API `/v1/responses`
- Anthropic-compatible `/v1/messages`
- Dynamic `/v1/models` backed by Verdent model discovery
- Native Verdent structured tool calling (not prompt-only emulation)
- Streaming text / thinking / tool events
- Multi-account routing with health, quota, cooldown and suspension states
- Per-account outbound proxy affinity
- Compatibility testing for OpenCode, Hermes Agent and Codex CLI

## Design principles

1. **Protocol first** — keep Verdent transport isolated from API compatibility layers.
2. **Native tools first** — structured tools/tool_choice are the primary path; text-contract parsing is fallback only.
3. **Dynamic catalog** — model capabilities must be discovered at runtime rather than hard-coded.
4. **Account state machine** — auth failure, rate limit, quota exhaustion and account suspension are different states.
5. **Stable egress** — an account can be pinned to one outbound proxy/transport.
6. **Clean-room implementation** — do not copy code from sources without compatible licensing.

## Planned architecture

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

## Roadmap

### Phase 0 — bootstrap
- repository conventions
- protocol notes and source-attribution record
- Go module + test harness
- CI

### Phase 1 — current Verdent protocol
- request envelope and AES-GCM codec
- headers/device metadata
- SSE parser
- runtime model catalog
- protocol fixtures

### Phase 2 — native agent/tool compatibility
- OpenAI tools -> Verdent structured tools
- Verdent tool events -> OpenAI/Anthropic tool calls
- thinking stream
- parallel tool calls
- tool result round-trip tests

### Phase 3 — API surfaces
- `/v1/chat/completions`
- `/v1/responses`
- `/v1/messages`
- `/v1/models`

### Phase 4 — account routing
- multi-account registry
- cooldown/rate-limit handling
- account suspension handling
- per-account proxy binding
- session/account affinity

### Phase 5 — integration verification
- OpenCode
- Hermes Agent
- Codex CLI
- long-running agent regression suite

## Status

Bootstrap started September 2026. The current Verdent desktop protocol must be verified against the latest client before declaring compatibility.
