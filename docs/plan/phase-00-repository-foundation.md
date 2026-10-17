# Phase 00 — Repository foundation

Status: complete.

## Goal

Make the repository describe and build one Go CLI for bare-metal OKD
installation assistance.

## Scope

- Establish the repository contract, architecture, specs, plan, and initial ADRs.
- Align the root README with the Go product and archive legacy Ansible usage.
- Add Cobra root/help/version commands and stable error handling.
- Pin the supported Go version in CI and the execution environment.

## Gate

- A contributor can find each durable fact through the docs map.
- `okub version` and root help work.
- Build, test, and vet pass in CI.
- No active documentation presents OCP or libvirt as a deployment target.

Dependencies: none.
