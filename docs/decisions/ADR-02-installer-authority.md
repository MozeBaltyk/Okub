# ADR-02: Keep openshift-install authoritative

Status: accepted.

## Context

Agent ISO and PXE formats change with OKD releases. Reimplementing installer
logic would duplicate upstream behavior and create a compatibility burden.

## Options

1. Delegate asset generation to a compatible pinned `openshift-install`.
2. Generate ignition and boot assets directly in Okub.
3. Maintain copied scripts for each supported release.

## Outcome

Okub generates and validates installer inputs, invokes `openshift-install`, and
verifies, records, and publishes its outputs. It does not implement installer
asset semantics.

Installer version compatibility is an explicit preflight/cache contract. Logs
are captured and redacted, and failures do not publish completed artifacts.
