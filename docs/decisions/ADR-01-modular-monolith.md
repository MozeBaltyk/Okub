# ADR-01: Use a Go modular monolith

Status: accepted.

## Context

Okub is a local orchestration tool with CLI/TUI entrypoints and several external
boundaries. The MVP does not need a server, database, plugin framework, or
independently deployed components.

## Options

1. A Go modular monolith with ports at external boundaries.
2. Multiple services with an API and persistent control plane.
3. Shell/Ansible orchestration as the primary implementation.

## Outcome

Use one Go binary. Keep domain rules and application workflows independent from
Cobra, Bubble Tea, process execution, and network adapters. Add narrow ports
only when a workflow needs them.

This keeps CLI and TUI behavior consistent and makes validation testable without
external infrastructure. A daemon or database requires a later ADR and a proven
need.
