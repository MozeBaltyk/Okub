# Phase 08 — Compact and HA MVP completion

Status: configuration, rendering, and CLI contracts complete; real artifact and
installation acceptance pending on a KVM-capable supported environment.

## Goal

Extend the validated bare-metal workflow from SNO to compact and HA topologies.

## Scope

- Add compact and HA topology rules and complete examples.
- Add load-balancer and multi-host DNS preflight requirements.
- Add renderer, compatibility, and negative fixtures for both topologies.
- Add disposable scenarios when dedicated-runner capacity permits.

## Gate

- Valid compact and HA definitions validate, render, and build.
- Bad host counts and network-service assumptions fail before artifact creation.
- SNO CLI and configuration contracts remain compatible.
- All MVP acceptance criteria in `specs/baseline.md` pass.

## Implemented contract coverage

- `examples/compact.yaml` covers three masters and no workers on platform
  `none`, with externally owned VIP, DNS, and load-balancer behavior.
- `examples/ha.yaml` covers three masters and two workers on platform
  `baremetal`, including installer-published API/ingress VIPs and PXE input.
- Schema, semantic, renderer, deterministic ordering/replica, CLI, and negative
  host-count/network-assumption tests cover both topologies.

The examples intentionally reference credential files and contain no credential
values. Real compact/HA ISO and PXE builds, physical/KVM boot, and completed
installations have not run in this environment because `/dev/kvm` is unavailable;
those phase and MVP gates remain pending.

Dependencies: Phases 05 and 06; Phase 07 validates the shared path.
