# Architecture

## Layers

### 1. API compatibility

Expose client-facing protocol surfaces without leaking Verdent-specific details:

- OpenAI Chat Completions
- OpenAI Responses
- Anthropic Messages
- Models

### 2. Canonical model

All client requests are converted into one internal representation:

- messages
- system blocks
- thinking
- tools and tool choice
- tool calls and tool results
- usage
- stream events

This prevents three separate API handlers from each implementing Verdent translation differently.

### 3. Verdent protocol core

Responsible only for:

- payload encoding/decoding
- request envelope construction
- header/device metadata
- upstream request/response handling
- SSE event parsing
- model catalog discovery

The protocol core must not know about HTTP dashboard or account-selection UI.

### 4. Account router

Each account owns its own transport and runtime state:

```text
Account
  |- credential reference
  |- device/session identity
  |- health state
  |- model cooldown state
  |- quota observations
  |- outbound transport
       |- direct
       |- HTTP proxy
       '- SOCKS proxy (later, if required)
```

A session should normally stay on the same account unless that account becomes unavailable.

### 5. API server

Thin handlers call the compatibility adapters and account router. They should not parse Verdent SSE directly.

## Tool calling

Primary path:

```text
client tools
 -> canonical tools
 -> Verdent structured tools
 -> upstream tool events
 -> canonical tool events
 -> client-native tool calls
```

Fallback text-contract parsing, if ever enabled, must be explicit and observable. It must never silently replace native tool calling.

## Version drift

Do not use a single hard-coded Verdent application version across the codebase. Runtime version/header values belong to protocol configuration and should be testable independently.
