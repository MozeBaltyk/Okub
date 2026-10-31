# Artifact-serving specification

Status: normative MVP target.

## Scope

Okub may serve a previously generated, verified ISO or PXE artifact set to bare-
metal hosts. Serving is explicit and ephemeral; it is not a permanent
provisioning service and is never started as a side effect of building files.

HTTP serves artifact content. Optional Pixiecore integration may provide PXE
discovery, ProxyDHCP, TFTP, and MAC-aware boot behavior where the existing site
network permits it.

## Inputs and interfaces

`okub artifacts serve` requires:

- a generated artifact-set manifest;
- an explicit bind address or interface and a host-reachable advertised address;
- an explicit serving mode (`http` or `pxe`);
- ports supplied by flags or documented defaults;
- host/MAC selection data already present in `okub.yaml` for PXE mode.

The command validates artifact paths, sizes, and SHA-256 checksums before
opening listeners. It rejects files outside the selected artifact directory,
symlink escapes, missing files, and a manifest from an unsupported version.

## Lifecycle

1. Load the cluster definition and artifact manifest.
2. Verify the complete artifact set without modifying it.
3. Validate bind addresses, ports, privileges, and mode-specific prerequisites.
4. Bind all required listeners or close every opened listener and fail.
5. Report readiness and endpoints only after every required listener is
   accepting traffic.
6. Serve in the foreground until cancellation or a required listener fails.
7. Close all listeners within a bounded shutdown period and remove only
   Okub-owned temporary state.

The serving lifecycle is `validating -> binding -> ready -> stopping ->
stopped`, with failure possible before or after readiness. An unexpected
failure of any required listener terminates the whole serving command; Okub
never leaves a partially functioning PXE service running.

Running in the background, installing a system service, or changing firewall
rules is outside the MVP. Those capabilities require a new specification.

## Network constraints

- Okub never starts a DHCP server in HTTP-only mode.
- PXE mode must not claim coexistence with an existing DHCP server until the
  selected Pixiecore/ProxyDHCP behavior has an integration test.
- A wildcard bind requires explicit user selection; it is never the implicit
  default.
- The command reports port conflicts before claiming readiness.
- The bind address is where local listeners accept traffic; the advertised
  address is what Okub places in iPXE, HTTP, rootfs, or kernel-argument URLs.
- A wildcard or loopback address is never advertised to physical hosts.
- The advertised address must belong to the selected interface and be
  reachable through the configured machine network. Ambiguous multi-homed
  configurations fail instead of relying on address autodetection.
- PXE ProxyDHCP supplies boot metadata only; it does not allocate host addresses
  or replace the site's DHCP service.

## PXE host admission and compatibility

- PXE boot instructions are available only to normalized MAC addresses present
  in the validated cluster definition. Unknown MAC addresses receive no boot
  specification and are reported as client rejections, not server failures.
- Duplicate DHCP, TFTP, HTTP, and iPXE requests from a configured host are
  expected and idempotent. A boot request never marks a host installed,
  consumed, or unavailable.
- Okub does not infer installation state from boot traffic or write discovered
  runtime values back to `okub.yaml`; `openshift-install` remains authoritative
  for installation progress.
- The initial PXE serving implementation supports x86-64 UEFI clients. An
  unsupported PXE architecture receives no boot specification and a diagnostic
  identifies the unsupported architecture. Other firmware and architectures
  require explicit compatibility evidence before being documented as supported.

## Privileges, provenance, and observability

- Okub validates all requested ports and required privileges before reporting
  readiness. It never invokes `sudo`; permission failures identify the listener
  and required operator action.
- The Pixiecore dependency and embedded or supplied iPXE binary are pinned.
  Their versions and the iPXE SHA-256 checksum are included in the serving
  result or associated run metadata. Changing that binary is a
  compatibility-sensitive change.
- Structured diagnostics distinguish startup failure, readiness, configured
  host match, unknown or unsupported client rejection, boot-file transfer
  failure, listener failure, and shutdown.
- Repeated successful file reads do not flood normal output. Requests and errors
  omit credential values and query secrets; JSON stdout remains one documented
  final result.

## Security and failure behavior

- No credential, generated secret, installer state, or arbitrary output-tree
  file is served.
- HTTP directory listing is disabled.
- Requests and errors are logged without query secrets or credential content.
- An invalid or modified artifact stops startup. HTTP serves private,
  digest-verified snapshots, so later source-file mutation cannot change bytes
  served by the active process and is rejected on the next startup.
- Cancellation follows Go context and leaves no permanent service, firewall
  rule, DHCP record, or network interface change.
- Serving boot artifacts does not create a target-side agent, callback API,
  per-host SSH installer, power-management service, or installation state
  machine.

## Acceptance

- Verified ISO and PXE sets are reachable from the selected test network.
- Missing, modified, symlink-escaped, and unsupported manifests are rejected.
- Binding is explicit and startup is all-or-nothing for required listeners.
- Ctrl-C produces a clean shutdown and stable exit code 130.
- HTTP mode coexists with existing DHCP without sending DHCP traffic.
- PXE mode serves configured MAC addresses idempotently, rejects unknown MACs
  and unsupported firmware, and does not allocate DHCP addresses.
- Wildcard, loopback, ambiguous, and unreachable advertised addresses are
  rejected before readiness.
- Failure of any required listener stops the serving unit and closes the rest.
- PXE coexistence is demonstrated in the disposable libvirt validation lab
  before it is marked supported.

Architecture: [bare-metal installation path](../architectures/baremetal/01-installation-path.md).
Plan: [Phase 06](../plan/phase-06-artifact-serving.md).
