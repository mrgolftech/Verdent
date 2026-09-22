# Desktop Identity & System Template Alignment

Status: **observed behavior, independently reproduced** (see Evidence). Records the
change set on top of `3ea35de` and the protocol mechanism behind it. Assumptions
are marked as such.

## 1. Context

The gateway mimics the Verdent **Desktop** client when talking to Verdent's LLM
proxy so that:

- requests are accepted by the proxy's routing/anti-abuse layer, and
- free/limited models (`*-free`) remain usable.

Upstream call: `POST https://llm-proxy.verdent.ai/llm/stream`.
Body is encrypted: AES-256-GCM, `key = base64(PROXY_SIGN)[:32]`, blob =
`base64(nonce12 | ciphertext | tag16)`. `system`, `messages` and `tools` are all
encrypted this way.

## 2. Mechanism — the `system` field is fingerprinted

Independently reproduced against the live proxy:

- The proxy **fingerprints the encrypted `system` field**. It must decrypt to the
  Desktop app's own **Main Agent prompt** (two required context blocks + a
  model/env block). Any replacement is routed into a strict rate lane and returns
  `error_code 20004` (`rate_limit_error`, HTTP 429, `need_retry:true`).
- Everything else (model, session/conv/react ids, messages, env, temperature,
  max_tokens) is **free-form**.
- Sustained mismatched traffic escalates from `20004` to the **account-level**
  `error_code 80006` ("free mode access has been suspended"). This is issued per
  account upstream and cannot be cleared locally.

Consequence: a captured Desktop `system` must be supplied via
`VERDENT_SYSTEM_TEMPLATE_FILE`. Without it the gateway encrypts its own system
blocks and lands in the `20004` lane.

## 3. Change set (this line)

| PR | Area | Change |
|----|------|--------|
| #1 | `internal/apikey`, `internal/server`, `internal/webui` | API key management: `GET/POST /api/keys`, `DELETE /api/keys/{id}`, admin "keys" page (create/copy/delete, no enable/disable). `/v1` auth accepts any managed key **or** the legacy `VERDENT_API_KEY`. |
| #2 | `internal/protocol`, `internal/config` | Desktop identity alignment: `VERDENT_OS_TYPE` / `VERDENT_DEVICE_TYPE` / `VERDENT_DEVICE_MODEL` override headers (Desktop sends `pc`/`windows`); envelope carries `thinking` (`{"type":"enabled","budget_tokens":4000}`) and `effort` (`high`) read from the template. |
| #3 | `internal/protocol`, `internal/config` | Populate upstream `env` (`platform` / `os_version` / `shell`) from the template and stamp `today_date`; previously an empty object was sent. |

All template-derived values are **read from the captured template**, never
hardcoded. When no template is configured, `thinking` is omitted and `env` stays
empty (previous behavior preserved).

## 4. Evidence (observed, not assumed)

Outgoing request captured on a local mock upstream, template configured:

- headers: `User-Agent: Verdent/2.15.1`, `X-Version-Code: 2.15.1`,
  `X-Os-Type: windows`, `X-Device-Type: pc`, `X-Team-Id: 0`,
  `X-Device-Id: <32-hex>`, `verdent-proxy-beta: hybrid-stream@20250919`,
  plus `Cookie` / `agent_type` / `CPU-Arch` (confirmed present in a real Desktop
  capture).
- body: `system` = captured ciphertext, `thinking`/`effort` present,
  `env` = `{platform:win32, os_version:..., shell:gitbash, today_date:<today>}`,
  `channel=deck`, `agent_name=VerdentDeck`, `temperature=1`, `max_tokens=64000`,
  `model_catalog_version` from template, `native_api=false`.

Behavioral results:

| Scenario | Result |
|----------|--------|
| no template, free model | `20004` rate lane (repeatedly) |
| template set, free model (temp instance) | **HTTP 200**, native `tool_calls`, full tool round-trip |
| account after `20004` storm | upstream `80006` (free mode suspended) |
| fresh account, free model | `30001` "Account is running out of credits" |
| `is_free`/`is_limit_free=true` for `*-free` | still `30001` — **ruled out** as the cause |

## 5. External references (versions dated, external, not a dependency)

- `thelabcorner/openfork` — `packages/opencode/src/plugin/verdent.ts` +
  `verdent-system.ts` (captured **2.12.3** Main Agent blocks, stored as
  plaintext, re-encrypted at runtime). Last touched 2026-09-22.
- `ErfanBagheri404/Verdent2API` — `template.json` (captured **2.15.1** encrypted
  `system` blob). Created 2026-09-22; template committed 2026-09-22. Self-declares
  unverified against the latest client.
- `PROXY_SIGN` is byte-identical across all three sources
  (`sha256[:16]=0adda0412ff380bb`), so a capture from any of them decrypts under
  the same key.
- The two captures **differ**: openfork = 2 blocks (no `You are Verdent…` intro);
  Verdent2API = 3 blocks (with intro). The gateway currently targets the **2.15.1**
  capture.

## 6. Open questions for review

1. **Target version.** The public release channel exposes 2.15.1 (2026-09-15) and
   2.16.0 (2026-09-22). Should the gateway re-capture against 2.16.0?
2. **`thinking`/`effort` on non-reasoning models** — currently sent for all
   models when a template provides them. Gate on model family instead?
3. **`is_free` / `is_limit_free`** — currently `false` (as in the 2.15.1 capture).
   Revisit if free routing needs the flag set for `*-free` models.
4. **Account-level errors (`80006` / `30001`)** are out of scope for this gateway:
   they are upstream account/quota state, not protocol shape.
