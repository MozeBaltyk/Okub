# Phase 07 — Libvirt validation lab

Status: planned.

## Goal

Validate the bare-metal ISO and PXE paths in disposable infrastructure. The lab
is a test harness, not a supported deployment target.

## Scope

- Implement the [lab specification](../specs/libvirt/validation.md).
- Add a minimal OpenTofu/libvirt network and SNO VM.
- Exercise the same artifacts and serving interfaces intended for physical hosts.
- Capture diagnostics and provide ownership-safe teardown.

## Gate

- Clean ISO and PXE scenarios reach the documented installation checkpoint.
- Cleanup removes only explicitly owned lab resources.
- Failure diagnostics retain no credentials.
- User-facing cluster configuration contains no libvirt platform mode.

Dependencies: Phases 05 and 06.
