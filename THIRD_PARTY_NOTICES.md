# Third-party references

This project is a clean-room implementation. Public projects may be used to understand interoperability behavior and to cross-check tests.

## OpenFork / OpenCode

- Repository: `thelabcorner/openfork`
- Relevant area: `packages/opencode/src/plugin/verdent.ts`
- License: MIT
- Copyright notice from the referenced repository: `Copyright (c) 2025 opencode`

The MIT license permits use, modification and distribution subject to retaining its copyright and permission notice in copies or substantial portions.


## ErfanBagheri404/Verdent2API

- Repository: `ErfanBagheri404/Verdent2API`
- Relevant areas: PKCE authentication flow, current Verdent 2.15.1 request-shape observations, message-block formatting, native tool passthrough, and burst-lane retry behavior
- License: MIT
- Referenced as an independent interoperability implementation; this project does not embed its captured opaque Desktop `system` blob

The implementation here keeps captured protocol material external and configurable rather than copying a version-specific request template into source control.

No user credentials, private captures, or proprietary Verdent application files are stored in this repository.
