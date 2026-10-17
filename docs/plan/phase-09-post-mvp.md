# Phase 09 — Post-MVP workstreams

Status: deferred.

## Goal

Track intentionally deferred capabilities without pulling them into the MVP.

## Candidate workstreams

- pfSense DNS, DHCP, VIP, firewall, and PXE rendering; apply remains separate.
- Disconnected release and mirror workflows.
- Additional CPU architectures after integration coverage exists.
- Bubble Tea TUI over existing application services.
- Multi-cluster profiles and optional run-history indexing.

## Entry gate

Each workstream requires its own normative spec and architecture impact review
before implementation. It may not bypass the CLI/application layer, duplicate
business rules, or weaken safety and secret-handling contracts.

Dependencies: Phase 08 unless a workstream's approved spec states otherwise.
