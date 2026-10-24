# Phase 05 — Agent boot artifacts

Status: automated contract implemented; real-artifact gate pending.

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

## Validation status

Focused tests cover archive extraction attacks, exact installer compatibility,
ISO/PXE output contracts, checksums and metadata, redaction, cancellation,
monitoring, CLI results, and atomic create/replace/rollback behavior without
network access. The common Go quality gate and focused race tests pass.

The phase remains incomplete until a supported environment uses the real cached
OKD installer and pinned release image to produce a non-empty SNO Agent ISO and
the complete PXE set. That network- and storage-intensive gate was not run as
part of the normal automated test suite.
