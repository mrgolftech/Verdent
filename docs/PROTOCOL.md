# Verdent Protocol Notes

This document records protocol behavior that has been independently verified for this project.

## Rules

- Do not commit access tokens, cookies, device identifiers or captured user content.
- Do not commit protocol secrets obtained from private credentials.
- Keep observed behavior separate from assumptions.
- Add a fixture/test for every protocol behavior we depend on.

## Verification matrix

| Area | Status | Evidence needed |
|---|---|---|
| Upstream endpoint | pending live verification | sanitized request metadata |
| Required headers | pending live verification | sanitized capture |
| Payload encryption | scaffolded | known-answer test fixture |
| Message schema | pending | synthetic + sanitized fixture |
| Native tools | pending | tool round-trip fixture |
| Thinking events | pending | SSE fixture |
| Model catalog | pending | sanitized response fixture |
| Rate limit semantics | pending | status/body fixture |
| Account suspension | pending | status/body fixture |

## Compatibility target

The project should track the current Verdent desktop protocol at release time. Historical client versions are references only; they are not treated as protocol truth.
