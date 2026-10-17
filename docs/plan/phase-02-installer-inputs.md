# Phase 02 — Deterministic installer inputs

Status: planned.

## Goal

Safely produce inspectable Agent-based Installer configuration.

## Scope

- Add typed `install-config.yaml` and `agent-config.yaml` renderers.
- Stage output, show intended changes, and publish atomically.
- Add `okub config render` and meaningful `--dry-run` output.
- Add deterministic golden fixtures for the SNO example.

## Gate

- Repeated rendering is byte-identical.
- Multi-file rendering cannot publish only one expected file.
- Existing files are not overwritten without explicit authorization.
- Sensitive generated files use restrictive permissions.

Dependencies: Phase 01.
