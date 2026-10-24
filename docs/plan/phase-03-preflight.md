# Phase 03 — Bare-metal preflight

Status: complete.

## Goal

Identify installation blockers before downloading or booting artifacts.

## Scope

- Check DNS, reverse DNS, API/API-int/apps records, addresses, VIPs, and routes.
- Check credentials, filesystem access, required tools, connectivity, and
  release-aware host resources.
- Add `okub preflight run` with aggregate human and JSON reports.

## Gate

- Independent checks continue after failures.
- Fixtures cover missing DNS, PTR mismatch, address conflicts, unreadable
  credentials, incompatible tools, and offline behavior.
- Preflight changes no host, network appliance, or cluster infrastructure.

Dependencies: Phase 01; it may consume Phase 02 output but must not require it.

## Delivered

- `okub preflight run` emits deterministic human or JSON results while running
  independent checks after individual failures.
- Direct configured-resolver A/AAAA and PTR checks cover API, API-int, apps, and
  static host names; read-only observations cover routes, addresses, VIP ports,
  credentials, output/cache access, installer/client compatibility, and endpoints.
  A complete verified cache satisfies tool compatibility without consulting
  `PATH` or probing metadata/tool-download endpoints; connected builds still
  probe the immutable-image registry.
- Explicit OKD 4.20 host-resource requirements avoid applying one resource rule
  to every release. Unknown release series fail closed until a compatibility
  rule is added.
- `--offline` performs no DNS, route, port, or HTTP probes and requires the
  checksum-verified installer/client/release metadata plus immutable OCI image metadata
  in the local cache.
- Fixture, unit, adapter, and CLI tests cover the phase gate failures, aggregate
  behavior, secret redaction, online success, and strict offline behavior.
