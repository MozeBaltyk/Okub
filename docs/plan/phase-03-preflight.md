# Phase 03 — Bare-metal preflight

Status: planned.

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
