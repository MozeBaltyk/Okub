# ADR-05: Keep boot transport separate from installation control

Status: accepted.

## Context

An ephemeral PXE server can grow into a provisioning control plane by adding a
target-side agent, callback API, per-host state, SSH installation, power
management, and discovered inventory updates. Those capabilities would overlap
with the Agent-based Installer, create another source of installation truth,
and expand Okub beyond local OKD installation orchestration.

## Options

1. Make Okub responsible for boot transport and per-host installation control.
2. Serve verified boot artifacts while leaving installation control to
   `openshift-install`.
3. Require an external provisioning platform for every PXE installation.

## Outcome

Okub may provide explicit ephemeral HTTP, ProxyDHCP, TFTP, and iPXE transport.
It records transport-level observations but does not introduce a target-side
agent, callback API, per-host SSH installation, power-management service, or
installation state machine. PXE requests are idempotent observations and never
prove that a host is installed.

`openshift-install` remains authoritative for installation progress and
completion. Serving and monitoring are independently cancellable workflows,
and serving does not mutate `okub.yaml` with discovered addresses or identity.
Observed state belongs in redacted runtime reports and logs.
