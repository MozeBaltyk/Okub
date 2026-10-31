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

A connected build with `openshift-install 4.20.0-okd-scos.17` and the pinned
release image produced a 924,307,456-byte SNO Agent ISO. Its published SHA-256
matched `run.json`, the release inputs remained verified cache hits, and
pull-secret material was absent from the published installer log and run
metadata.

The phase remains incomplete until a KVM-capable supported environment produces
and verifies the complete PXE set. The current WSL2 validation host does not
expose hardware virtualization to libvirt, so its Fedora prerequisite helper
cannot start as a KVM domain; software-emulated TCG is intentionally not added
to the supported helper contract solely to bypass this gate.
