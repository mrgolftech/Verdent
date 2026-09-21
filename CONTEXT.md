# Project Context

## Objective

Build a maintainable Verdent API compatibility gateway for OpenCode, Hermes Agent and Codex CLI.

## Current decisions

- Language: Go.
- Implementation style: clean-room.
- Protocol and compatibility layers stay separate.
- Native structured tool calling is mandatory; prompt-based tool emulation is fallback only.
- Model catalog is runtime-discovered.
- Account routing distinguishes authentication failure, rate limiting, quota exhaustion and suspension.
- Per-account proxy affinity is part of the account transport abstraction, not a global process proxy.
- Credentials and protocol-sensitive values must not be committed.

## Reference projects

- D3-vin/Verdent2Api: useful product/API surface reference; licensing is not clear enough to copy source.
- thelabcorner/openfork: MIT-licensed; useful reference for Verdent protocol/tool/event behavior.
- st0xr0/verdent2api: useful historical reference for local Agent/sidecar event structure.

## Immediate work

1. Establish protocol config and codec tests.
2. Add canonical streaming event model.
3. Add Verdent SSE parser using captured/synthetic fixtures.
4. Add API compatibility adapters.
5. Add account router and per-account transports.
6. Verify against the current Verdent client before tagging a release.
