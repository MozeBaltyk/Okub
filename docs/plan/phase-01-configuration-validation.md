# Phase 01 — Configuration and validation

Status: planned.

## Goal

Establish the versioned `okub.yaml` source-of-truth contract.

## Scope

- Add `ClusterDefinition` domain types and `okub.io/v1alpha1` JSON Schema.
- Load YAML strictly, apply documented defaults, and reject unknown fields.
- Aggregate deterministic structural, topology, and network findings.
- Add `okub config validate`, one valid SNO example, and negative fixtures.

## Gate

- Tests cover missing fields, malformed names/addresses, duplicates, CIDR
  overlap, address conflicts, invalid host prefixes, and bad SNO host counts.
- Independent errors are reported together in human and JSON formats.
- Credentials are absent from reports and test output.

Dependencies: Phase 00.
