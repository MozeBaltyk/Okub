# Phase 04 — OKD release resolution and cache

Status: planned.

## Goal

Acquire exactly the installer and image inputs required by the requested OKD
release.

## Scope

- Resolve OKD release and installer compatibility.
- Add content-addressed caching and atomic insertion.
- Verify every digest/checksum and implement strict offline behavior.
- Add `okub images sync`.

## Gate

- Verified cache hits perform no download.
- Truncated, corrupt, or mismatched content is never published.
- Concurrent downloads cannot corrupt a cache entry.
- Tests use local fixture servers rather than public endpoints.

Dependencies: Phases 01 and 03.
