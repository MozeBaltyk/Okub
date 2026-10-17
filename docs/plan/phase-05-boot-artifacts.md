# Phase 05 — Agent boot artifacts

Status: planned.

## Goal

Complete the first useful end-to-end path by producing verified Agent ISO and
PXE artifacts.

## Scope

- Add the `openshift-install` adapter with version checks and cancellation.
- Add `okub artifacts build --type iso|pxe`.
- Publish checksummed artifacts, redacted logs, and immutable run metadata.
- Add `okub install monitor`.

## Gate

- The SNO example produces a non-empty Agent ISO and a complete PXE artifact set.
- Failure or cancellation publishes no completed-looking artifact.
- Pull-secret values do not appear in arguments, logs, reports, or errors.
- Run metadata identifies every input needed to reproduce the build.

Dependencies: Phases 02, 03, and 04.
