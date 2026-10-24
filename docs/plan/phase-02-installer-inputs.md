# Phase 02 — Deterministic installer inputs

Status: complete.

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

## Delivered

- Typed, deterministic Agent-based Installer configuration for the supported
  SNO example, including normalized static networking and rendezvous IP.
- Bounded credential-file reads and secret-free human/JSON command results.
- Same-filesystem staging, atomic generated-directory publication, restrictive
  file permissions, dry-run change reporting, and explicit forced replacement.
- Golden, workflow, filesystem-failure, permission, and CLI contract tests.
