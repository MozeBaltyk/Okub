# ADR-03: Use libvirt only for validation

Status: accepted.

## Context

Automated ISO and PXE boot testing needs disposable machines and networks, but
Okub's product goal is installation on physical bare metal.

## Options

1. Treat libvirt as a supported deployment platform.
2. Use libvirt only as a harness for the bare-metal workflow.
3. Test exclusively on physical hardware.

## Outcome

Use OpenTofu/libvirt only for disposable integration and end-to-end validation.
The lab consumes normal bare-metal configuration and artifacts. It must not add
a libvirt platform to `okub.yaml` or maintain a separate installation path.

Physical hardware remains necessary for final compatibility evidence where VM
firmware or networking cannot represent real equipment.
