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
- ErfanBagheri404/Verdent2API: MIT-licensed September 22 reference with current Verdent 2.15.1 PKCE, captured Desktop request-shape, message-block and burst-lane findings; use as evidence/reference, not as a reason to hard-code its opaque system blob.

## Immediate work

1. Establish protocol config and codec tests.
2. Add canonical streaming event model.
3. Add Verdent SSE parser using captured/synthetic fixtures.
4. Add API compatibility adapters.
5. Add account router and per-account transports.
6. Verify against the current Verdent client before tagging a release.


## 2026-09-22 live-protocol alignment audit

Implemented on `fix/live-protocol-alignment`:

1. PKCE browser login now matches the observed production shape, validates OAuth state, parses user/expiry/refresh fields, persists refresh credentials, and supports refresh-token rotation.
2. Captured Desktop template mode preserves the encrypted `system` fingerprint and folds downstream system instructions into the first user turn.
3. Upstream messages now use Desktop-shaped block arrays with timestamp/cache-control metadata; only assistant turns carry `model`.
4. `native_api` is no longer hard-coded true; current captured default is false and remains explicitly overridable.
5. Verdent upstream pacing/retry is per account, preserving multi-account parallelism and fixed egress affinity.

Remaining verification boundary: perform one real browser login and one real agent/tool conversation against the current Verdent production service before release/merge.
