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
- an explicit bind address or interface;
- an explicit serving mode (`http` or `pxe`);
- ports supplied by flags or documented defaults;
- optional host/MAC selection data already present in `okub.yaml`.

The command validates artifact paths, sizes, and SHA-256 checksums before
opening listeners. It rejects files outside the selected artifact directory,
symlink escapes, missing files, and a manifest from an unsupported version.

## Lifecycle

1. Load the cluster definition and artifact manifest.
2. Verify the complete artifact set without modifying it.
3. Validate bind addresses, ports, privileges, and mode-specific prerequisites.
4. Bind all required listeners or fail without serving.
5. Report endpoints to stdout or as one JSON result.
6. Serve in the foreground until cancellation or failure.
7. Close listeners and remove only Okub-owned temporary state.

Running in the background, installing a system service, or changing firewall
rules is outside the MVP. Those capabilities require a new specification.

## Network constraints

- Okub never starts a DHCP server in HTTP-only mode.
- PXE mode must not claim coexistence with an existing DHCP server until the
  selected Pixiecore/ProxyDHCP behavior has an integration test.
- A wildcard bind requires explicit user selection; it is never the implicit
  default.
- The command reports port conflicts before claiming readiness.
- URLs emitted into boot arguments use the address reachable by target hosts,
  not an unrelated wildcard or loopback address.

## Security and failure behavior

- No credential, generated secret, installer state, or arbitrary output-tree
  file is served.
- HTTP directory listing is disabled.
- Requests and errors are logged without query secrets or credential content.
- An invalid or modified artifact stops startup; a file becoming invalid during
  service is treated as an error, not silently served.
- Cancellation follows Go context and leaves no permanent service, firewall
  rule, DHCP record, or network interface change.

## Acceptance

- Verified ISO and PXE sets are reachable from the selected test network.
- Missing, modified, symlink-escaped, and unsupported manifests are rejected.
- Binding is explicit and startup is all-or-nothing for required listeners.
- Ctrl-C produces a clean shutdown and stable exit code 130.
- HTTP mode coexists with existing DHCP without sending DHCP traffic.
- PXE coexistence is demonstrated in the disposable libvirt validation lab
  before it is marked supported.

Architecture: [bare-metal installation path](../architectures/baremetal/01-installation-path.md).
Plan: [Phase 06](../plan/phase-06-artifact-serving.md).
