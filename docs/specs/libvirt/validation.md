# Libvirt validation-lab specification

Status: normative MVP test contract.

## Purpose

The libvirt lab validates the same OKD configuration, boot artifacts, and
serving behavior intended for physical bare-metal hosts. Passing the lab is
evidence about that workflow; it does not make libvirt an Okub deployment
platform.

## Scope

- One disposable isolated network with deterministic addressing.
- One SNO VM first; compact and HA scenarios may reuse the harness later.
- ISO boot and PXE boot of artifacts produced by Okub.
- Existing-network-service simulation sufficient for DNS, DHCP, and routing.
- Installation checkpoint observation and diagnostic collection.
- Idempotent, ownership-safe teardown.

The lab must not introduce `platform: libvirt` into `okub.yaml`, maintain a
separate rendering path, or use lab-only boot artifacts.

## Inputs and outputs

Inputs are a normal bare-metal cluster definition, its generated artifact
manifest, and lab resource sizing. Lab-specific state stays under the ignored
lab working directory, never in the cluster definition.

Outputs are scenario status, redacted installer logs, timestamps, and resource
identifiers needed for teardown. Raw serial output is not persisted because it
may contain credential values without identifying labels; `serial.log` records
that omission. These outputs are test evidence rather than user deployment
artifacts.

## Lifecycle

1. Verify host virtualization and required tool prerequisites.
2. Create uniquely owned network and VM resources.
3. Attach the published ISO or boot through the normal artifact server.
4. Wait for the phase-specific OKD installation checkpoint.
5. Capture redacted diagnostics on success or failure.
6. Destroy only resources carrying the run's ownership identifier.

Cleanup must be safe to rerun. A failed create may leave diagnostics but must
not make unrelated libvirt networks or guests eligible for deletion.

## Acceptance

- A clean SNO ISO scenario reaches the documented checkpoint.
- A clean SNO PXE scenario exercises the production serving interface and
  reaches the same checkpoint. The scenario proves that the existing DHCP
  service allocates the address, ProxyDHCP supplies only boot metadata, the
  configured MAC is selected, and the VM requests the published iPXE program,
  kernel, initramfs, and rootfs reference before the Agent environment starts.
- Repeated PXE requests from the configured host remain successful and do not
  consume or change host state.
- Unknown MAC addresses and unsupported PXE architectures receive no boot
  specification; a modified manifest or missing boot file prevents readiness.
- Port conflicts fail before readiness, and failure of one required listener
  closes the remaining listeners.
- Interrupting the server produces bounded cleanup, after which the existing
  DHCP service remains operational.
- A multi-interface serving-host scenario rejects an ambiguous or unreachable
  advertised address and succeeds when bind interface and advertised address
  are explicit.
- Repeated setup/teardown does not collide with another run.
- Teardown selects resources by explicit ownership, not name prefix alone.
- Logs and state contain no pull-secret or private-key content.
- No user-facing command or config describes libvirt as an installation target.

Architecture: [validation lab](../../architectures/libvirt/01-validation-lab.md).
Plan: [Phase 07](../../plan/phase-07-libvirt-validation.md).
