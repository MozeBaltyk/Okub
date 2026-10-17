# General architecture

Status: target architecture for the Go implementation.

## 1. Architectural style

Okub is a local, Go-based modular monolith for bare-metal OKD installation. It uses ports and adapters where
Okub crosses a process, network, or filesystem boundary. It is not a service:
the MVP has no daemon, API server, plugin system, or required database.

The CLI and TUI are delivery mechanisms over the same application services.
Business rules must not be implemented in Cobra commands, Bubble Tea models,
shell scripts, or infrastructure adapters.

```text
                 CLI (Cobra)          TUI (Bubble Tea)
                       \                 /
                        application workflows
             prepare / validate / build / monitor / diagnose
                                  |
                         domain model and rules
                                  |
                                ports
             +--------------------+--------------------+
             |          |         |        |           |
       filesystem   installer    DNS      HTTP      virtualization
             |          |         |        |           |
         local OS  openshift-   resolver  release    libvirt /
                   install                metadata   OpenTofu
```

Dependencies point inward: adapters depend on ports and domain types; the
domain never imports adapters, CLI, TUI, Cobra, or operating-system execution
packages.

## 2. System boundary

Okub owns:

- loading and validating the declarative cluster definition;
- deterministic generation of installer inputs;
- preflight checks and aggregated reports;
- acquisition and verification of compatible OKD tools and images;
- orchestration of `openshift-install`;
- local artifact, log, and run metadata management;
- ephemeral artifact serving;
- disposable libvirt orchestration used only to validate the bare-metal path.

Okub does not own:

- OKD installation algorithms or installer asset formats;
- permanent DHCP, DNS, PXE, registry, or firewall services;
- ongoing cluster administration;
- silent mutation of routers, physical hosts, or firewall configuration.

`openshift-install` remains authoritative for Agent ISO and PXE assets. Okub
generates its inputs, invokes it, and records its outputs.

## 3. Repository modules

```text
cmd/okub/                     process entrypoint and dependency wiring
internal/domain/              models, value validation, invariants
internal/application/         workflows and application result types
internal/ports/               interfaces for external capabilities
internal/adapters/
  filesystem/                 atomic writes and output layout
  openshift/                  installer discovery, execution and monitoring
  coreos/                     release metadata, downloads and cache
  dns/                        forward/reverse DNS checks
  libvirt/                    lab lifecycle integration
  pfsense/                    future config renderer/apply adapter
  pxe/                        optional ephemeral PXE serving adapter
internal/cli/                 Cobra commands and output presentation
internal/tui/                 Bubble Tea presentation
schemas/                      versioned Okub JSON schemas
templates/                    text templates only where typed YAML is unsuitable
examples/                     complete supported cluster definitions
lab/terraform/libvirt/        disposable integration environment
tests/                        integration, end-to-end, fixtures and scenarios
```

Packages are added when a supported workflow needs them. Empty interfaces and
general-purpose frameworks are not part of the architecture.

## 4. Domain model

The root aggregate is `ClusterDefinition`, loaded from `okub.yaml`. It contains:

- identity and requested OKD release;
- topology and platform;
- machine, service, and cluster networks;
- hosts, roles, interfaces, addresses, and installation disks;
- boot and artifact policies;
- references to credentials, never copied secret values;
- optional platform-specific settings.

Important supporting types are:

- `ValidationReport`: ordered errors, warnings, check identifiers, and hints;
- `Release`: OKD version, architecture, image digest, and installer source;
- `Artifact`: type, path, size, checksum, and producing tool;
- `Run`: immutable input digest, tool versions, times, outcome, and artifacts;
- `CommandResult`: redacted command metadata, exit status, and log location.

Domain validation is pure and deterministic. Checks requiring DNS, HTTP,
process execution, or filesystem state belong to preflight application logic
behind ports.

## 5. Application workflows

### Validate configuration

1. Read the requested configuration file.
2. Decode YAML while rejecting unknown fields.
3. Validate it against its versioned schema.
4. Apply defaults defined by that schema version.
5. Run semantic and cross-field validation.
6. Return one ordered report containing every detected issue.

### Render installer inputs

1. Complete configuration validation.
2. Build typed `install-config` and `agent-config` representations.
3. Serialize them deterministically into a staging directory.
4. Compare with existing output and present the proposed changes.
5. Publish with atomic rename only after the overwrite policy is satisfied.

### Run preflight

1. Complete configuration validation.
2. Resolve the requested release and compatible installer.
3. Run independent host, network, DNS, credential, and filesystem checks.
4. Aggregate results without changing infrastructure.
5. Write human-readable and JSON reports.

### Build an Agent ISO

1. Complete validation and preflight.
2. Resolve and verify the release and installer.
3. Render installer inputs into a private staging directory.
4. Invoke `openshift-install agent create image`.
5. Verify the output and calculate its checksum.
6. Atomically publish the artifact, logs, and run manifest.

Failure leaves existing published artifacts unchanged. Temporary secrets and
staging data are removed on normal exit and use restrictive permissions.

### Build and serve PXE assets

1. Complete the same validation, preflight, and release-resolution steps.
2. Ask `openshift-install` to produce the authoritative PXE files.
3. Verify and publish the kernel, initramfs, rootfs reference, and boot manifest.
4. Serve them only when explicitly requested, in the foreground by default.
5. Stop cleanly without installing a permanent network service.

Simple artifact delivery uses HTTP. Network boot discovery/TFTP uses the
optional Pixiecore adapter and must be tested for coexistence with the site's
existing DHCP service.

## 6. Ports and adapters

Ports should be narrow and defined by application needs. Expected capabilities
include:

| Port | Responsibility | Initial adapter |
|---|---|---|
| Configuration store | Read config and publish generated files | Local filesystem |
| Installer | Report version, create assets, monitor install | `os/exec` |
| Release resolver | Resolve release metadata and compatible downloads | HTTPS |
| Artifact cache | Content-addressed get/put with checksum verification | Local filesystem |
| DNS checker | Query A/AAAA/PTR records against configured resolvers | `miekg/dns` |
| Artifact server | Ephemerally expose generated ISO/PXE assets | HTTP/Pixiecore |
| Clock | Supply timestamps for testable run metadata | System clock |
| Lab | Create, inspect, and destroy a disposable environment | OpenTofu/libvirt |

The filesystem adapter is a security boundary. It must prevent output path
escape, avoid following unsafe replacement paths, set appropriate permissions,
and use same-filesystem staging for atomic publication.

## 7. State and output

Okub separates three forms of state:

1. **Desired state:** user-owned `okub.yaml` and referenced credentials.
2. **Generated state:** inspectable installer inputs and boot artifacts.
3. **Runtime state:** reports, command logs, progress, and run metadata.

Default output layout:

```text
build/<cluster>/
├── generated/
│   ├── install-config.yaml
│   └── agent-config.yaml
├── artifacts/
│   ├── agent.<architecture>.iso
│   └── pxe/
│       ├── kernel
│       ├── initramfs.img
│       └── manifest.json
├── reports/
│   ├── validation.json
│   └── preflight.json
├── logs/
│   └── openshift-install.log
└── run.json
```

Downloads are stored separately in a content-addressed cache. The cache is
reusable; a build directory is specific to one cluster definition and run.
SQLite may be considered later for querying run history, but files are the MVP
system of record.

## 8. Security and safety

- Pull secrets and private keys are referenced, not embedded in configuration.
- Secret values are never included in command strings, reports, or logs.
- Sensitive generated files use mode `0600`; normal artifacts use `0644` only
  when their content is safe to expose.
- Downloaded binaries and images require a verified digest or checksum.
- External binaries are pinned and their versions are recorded.
- Commands use argument arrays rather than shell interpolation.
- Existing files are never overwritten silently.
- Infrastructure-changing actions require explicit commands and confirmation;
  `--dry-run` never performs the action.
- Applying pfSense configuration remains separate from rendering it.

## 9. Concurrency and failure handling

One process owns a cluster output directory at a time using a lock file. Cache
entries may be downloaded concurrently by staging to unique temporary names
and atomically publishing the verified result.

External command cancellation follows the Go context. Okub preserves redacted
diagnostic logs, reports a stable error category, and removes incomplete
published artifacts. Retrying an operation must not corrupt prior output.

## 10. Deployment model

The primary distribution is one static Go binary. An optional execution
environment image supplies tools that are difficult to install consistently.
The binary behaves the same on the host and in that image; scripts and the
`justfile` only invoke supported CLI workflows.

The libvirt lab is development and integration-test infrastructure that
simulates the bare-metal boot path. It is not a supported installation target
or runtime dependency for physical bare-metal deployments.

## 11. Testing boundaries

- Domain tests cover pure validation and topology/network invariants.
- Application tests use small fake ports and verify workflow behavior.
- Adapter tests cover process invocation, DNS, downloads, checksums, and atomic
  filesystem behavior.
- Contract tests pin schema and CLI JSON compatibility.
- Integration tests exercise the real installer without requiring a cluster.
- End-to-end tests use disposable libvirt infrastructure on dedicated runners.

Scoped views:

- [bare-metal installation path](baremetal/01-installation-path.md)
- [libvirt validation lab](libvirt/01-validation-lab.md)

See [the baseline specification](../specs/baseline.md) for observable behavior
and [the delivery plan](../plan/summary.md) for implementation order.
