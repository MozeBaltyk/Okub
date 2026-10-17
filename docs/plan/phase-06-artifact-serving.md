# Phase 06 — Ephemeral artifact serving

Status: planned.

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
