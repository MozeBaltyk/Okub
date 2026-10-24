# Phase 04 — OKD release resolution and cache

Status: complete.

## Goal

Acquire exactly the installer, client, and image inputs required by the requested OKD
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

## Delivered

- `okub images sync` deterministically resolves an exact release and compatible
  installer, then emits secret-free human or JSON results.
- The resolver selects the newest naturally ordered `Accepted` tag matching the
  requested series from the public `4-scos-stable` release-controller stream,
  resolves that tag's release-detail digest, converts the matching tagged image
  reference into an immutable digest pullspec, then selects the exact tag's
  Linux installer, Linux client, and `release.txt` assets from the GitHub
  releases API. All HTTP assets must carry SHA-256 digests.
- Installer, client, and release metadata are streamed into same-filesystem staging,
  checked against declared size and SHA-256 digest, and atomically inserted by
  digest before a deterministic release manifest is published. The release's
  immutable OCI pullspec and image digest are retained in that manifest.
- Verified release manifests and blobs are re-verified on lookup. A complete hit
  performs no metadata request or artifact download; corrupt and partial entries
  are never reused.
- Concurrent syncs safely converge on the same blobs and manifest. Strict
  offline mode performs no network operation and succeeds only on a fully
  verified cache hit.
- Phase 03 preflight uses this same cache lookup contract; its temporary
  per-release file convention was removed.
- Verified `openshift-install` and `oc` executables are safely materialized under
  `./.local/tools/<exact-release>/`; preflight does not require `PATH` tools or
  probe their download endpoints after a complete cache hit.
- Disconnected installation remains outside MVP scope: the cache does not claim
  to contain OCI image layers. Offline sync/preflight make no network calls and
  verify local HTTP inputs and immutable image metadata; a later connected build
  may pull the pinned OCI image by digest.
- Local HTTP fixture, corruption, truncation, mismatch, concurrency, offline,
  incompatibility, unavailable-release, CLI, and focused race tests cover the
  phase gate.
