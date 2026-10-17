# Phase 08 — Compact and HA MVP completion

Status: planned.

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

Dependencies: Phases 05 and 06; Phase 07 validates the shared path.
