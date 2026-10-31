# Phase 06 — Ephemeral artifact serving

Status: core serving contract complete; structured per-client observability,
privileged ProxyDHCP coexistence, and real PXE boot validation remain pending.

## Goal

Serve generated artifacts explicitly without creating a permanent provisioning
service or changing the site's network infrastructure.

## Scope

- Implement the [artifact-serving specification](../specs/artifact-serving.md).
- Add `okub artifacts serve` with manifest verification and explicit binding.
- Serve content over HTTP and add optional Pixiecore PXE discovery/TFTP.
- Test coexistence with an existing DHCP service.

## Gate

- Serving never starts as a build side effect.
- Modified or incomplete artifact sets are rejected.
- Shutdown leaves no installed service or persistent network change.
- ProxyDHCP behavior is tested before it is documented as supported.

Dependencies: Phase 05.

## Implemented contract coverage

- `okub artifacts serve` verifies the published manifest before binding and
  serves only its allowlisted artifacts over explicit HTTP.
- Optional Pixiecore ProxyDHCP/TFTP accepts only configured host MAC addresses,
  supports x86_64 UEFI, and preserves the installer-generated iPXE script and
  architecture-specific artifact names.
- PXE startup verifies that `boot.artifactsBaseURL` exactly matches the
  advertised HTTP endpoint; cancellation stops all listeners without installing
  a service or changing persistent network configuration.
- Unit and integration contracts cover modified/incomplete sets, address and
  endpoint validation, MAC admission, listener supervision, and shutdown.

The remaining gate requires a privileged network and KVM-capable runner to
demonstrate coexistence with a real DHCP service and boot a real SNO PXE set.
