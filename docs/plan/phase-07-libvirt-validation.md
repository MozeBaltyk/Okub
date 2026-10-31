# Phase 07 — Libvirt validation lab

Status: harness and safety contracts complete; KVM-backed ISO/PXE installation
checkpoints and teardown verification remain pending.

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

## Implemented contract coverage

- `okub lab up|test|down` uses the normal bare-metal configuration and verified
  published artifacts; no libvirt platform mode or lab-only boot path exists.
- The OpenTofu harness creates one isolated SNO network, root volume, and domain
  with an explicit run UUID and defaults to inspected plans unless `--apply` is
  supplied.
- Apply and destroy reject replacement, unexpected resources, mismatched live
  identities or attachments, and foreign ownership. Diagnostics are private and
  bounded; arbitrary serial content is fully withheld because it cannot be
  reliably scrubbed of credentials.
- OpenTofu formatting, validation, and the mock-provider PXE plan pass; the CLI
  rejects this host before mutation because usable KVM is unavailable.

Real ISO/PXE boots, installation checkpoint evidence, and repeated exact-owner
teardown must be run on a KVM-capable supported environment.
