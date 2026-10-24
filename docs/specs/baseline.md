# Baseline product and behavior specification

Status: normative target for the first supported releases. Where this document
and general prose differ, this document defines observable behavior.

## 1. Product goal

Okub helps an operator install one OKD cluster on bare metal from one
declarative definition. It checks that prerequisites are fulfilled, generates
and validates installer configuration, produces boot artifacts, and can serve
those artifacts. It uses the Agent-based Installer rather than reproducing
installer logic.

Libvirt is test infrastructure used to validate this bare-metal workflow. It is
not a supported deployment target.

The first usable slice is one x86_64 single-node cluster built as an Agent ISO.
The MVP then adds PXE generation/serving, compact and HA topologies, and a
disposable libvirt validation scenario.

## 2. Scope

### MVP

- One cluster definition per command invocation.
- OKD release selection.
- `x86_64` architecture.
- SNO, compact, and HA topologies.
- `none` and `baremetal` Agent-based Installer platforms where supported.
- Static addressing or an existing DHCP service.
- Deterministic `install-config.yaml` and `agent-config.yaml` generation.
- Configuration and environmental preflight validation.
- Release-compatible installer and image acquisition with checksum validation.
- Agent ISO and PXE generation, optional ephemeral serving, and installation monitoring.
- Artifact serving follows the separate [serving contract](artifact-serving.md).
- A disposable libvirt lab follows the [validation contract](libvirt/validation.md)
  and validates the bare-metal path.
- Human-readable and JSON output.

### After MVP

- Fedora helper artifact synchronization and readiness automation beyond the
  implemented image, cloud-init, and guarded lifecycle baseline.
- Disconnected installation.
- Additional CPU architectures.
- Multi-cluster profiles and local run-history queries.
- Interactive TUI.

### Excluded

- Reimplementation of `openshift-install`.
- A permanent provisioning control plane.
- Automatic router or firewall mutation as part of build commands.
- Day-two cluster administration.

## 3. Canonical configuration

The default filename is `okub.yaml`. Documents use an explicit API version:

```yaml
apiVersion: okub.io/v1alpha1
kind: Cluster

metadata:
  name: lab

cluster:
  baseDomain: example.test
  version: "4.20"
  architecture: x86_64
  topology: sno

platform:
  type: none

network:
  machineCIDR: 192.168.100.0/24
  clusterCIDR: 10.128.0.0/14
  hostPrefix: 23
  serviceCIDR: 172.30.0.0/16
  dnsServers:
    - 192.168.100.1
  gateway: 192.168.100.1
  apiVIP: 192.168.100.10
  ingressVIP: 192.168.100.11
  rendezvousIP: 192.168.100.20

hosts:
  - name: master-0
    role: master
    interfaces:
      - name: eno1
        macAddress: "52:54:00:00:00:10"
    address: 192.168.100.20/24
    installationDisk: /dev/vda

boot:
  method: iso

artifacts:
  outputDirectory: ./.local/build/lab
  cacheDirectory: ./.local/cache
  checksumPolicy: required

credentials:
  pullSecretFile: ~/.config/okub/pull-secret.json
  sshPublicKeyFile: ~/.ssh/id_ed25519.pub
```

### 3.1 Field contract

| Field | Required | Contract |
|---|---:|---|
| `apiVersion` | yes | Exactly `okub.io/v1alpha1` for this schema |
| `kind` | yes | Exactly `Cluster` |
| `metadata.name` | yes | DNS label, used in output paths and cluster identity |
| `cluster.baseDomain` | yes | Valid DNS domain without a wildcard or trailing dot |
| `cluster.version` | yes | Release channel/version accepted by the resolver; quote YAML values |
| `cluster.architecture` | yes | `x86_64` in MVP |
| `cluster.topology` | yes | `sno`, `compact`, or `ha` |
| `platform.type` | yes | `none` or `baremetal` in MVP; SNO requires `none` |
| `network.machineCIDR` | yes | IPv4 CIDR containing host and applicable VIP addresses |
| `network.clusterCIDR` | yes | Pod network CIDR |
| `network.hostPrefix` | yes | Node subnet prefix within `clusterCIDR` |
| `network.serviceCIDR` | yes | Service network CIDR |
| `network.dnsServers` | yes | One or more resolver IP addresses |
| `network.gateway` | conditional | Required for statically addressed hosts |
| `network.apiVIP` | conditional | Required where the selected platform/topology uses a VIP |
| `network.ingressVIP` | conditional | Required where the selected platform/topology uses a VIP |
| `network.rendezvousIP` | conditional | IPv4 address in `machineCIDR`; required when no master has a static address |
| `hosts` | yes | Non-empty list satisfying topology rules |
| `hosts[].name` | yes | Unique DNS label |
| `hosts[].role` | yes | `master` or `worker` |
| `hosts[].interfaces` | yes | At least one uniquely named interface; a statically addressed host currently requires exactly one |
| `interfaces[].macAddress` | yes | Globally unique unicast MAC address |
| `hosts[].address` | conditional | Static address with prefix; omit when using DHCP |
| `hosts[].installationDisk` | yes | Absolute installer-visible device path under `/dev`, including stable `/dev/disk/by-*` paths |
| `boot.method` | yes | `iso` or `pxe` |
| `artifacts.outputDirectory` | no | Defaults to `./.local/build/<metadata.name>` |
| `artifacts.cacheDirectory` | no | Defaults to `./.local/cache` |
| `artifacts.checksumPolicy` | no | Defaults to `required`; it cannot be disabled in MVP |
| `credentials.pullSecretFile` | yes | Readable regular file; content is never copied to reports |
| `credentials.sshPublicKeyFile` | yes | Readable OpenSSH public key file |
| `helper` | no | Optional Fedora Cloud 44 network-services VM desired state |
| `helper.fedoraRelease`, `vmName`, `storagePool` | with `helper` | Release is fixed to 44; domain and existing active pool are explicit |
| `helper.wanCIDR` | no | Dedicated RFC1918 `/24`-`/29` NAT WAN; defaults to `172.31.255.0/24` and must not overlap configured or active networks |
| `network.machineCIDR` with `helper` | yes | Managed LAN is `/24`-`/29`; helper uses first usable and host bridge reserves final usable |
| `helper.wanMAC`, `lanMAC` | with `helper` | Distinct locally administered unicast MAC addresses |
| `helper.libvirtURI`, `vcpu`, `memoryMiB`, `rootDiskSizeGiB` | no | URI is fixed to local `qemu:///system`; resource defaults are 2, 2048 MiB, and 20 GiB |

Unknown fields are errors. Environment expansion inside YAML values is not
performed. Leading `~/` is resolved only for artifact directories and local
credential files. Installation disks are target-host identifiers and are never
expanded as local paths.

### 3.2 Defaults

Defaults are versioned and visible in rendered output:

- output directory: `./.local/build/<cluster-name>`;
- cache directory: `./.local/cache`;
- checksum policy: `required`;
- rendezvous IP: explicit `network.rendezvousIP` when set; otherwise the address
  of the lexically first statically addressed master after hosts are normalized.

Okub does not guess networks, host IP addresses, installation disks, release
versions, or credentials.

## 4. Validation requirements

Validation returns all findings in deterministic order. A finding contains a
stable check ID, severity, configuration path, message, and optional hint.

### 4.1 Structural checks

- Supported API version and kind.
- Required fields and allowed values.
- Valid DNS names, CIDRs, IP addresses, MAC addresses, and paths.
- No unknown fields.
- Unique host names, IP addresses, and MAC addresses.

### 4.2 Network checks

- Machine, cluster, and service CIDRs do not overlap.
- `hostPrefix` is more specific than `clusterCIDR` and can accommodate the
  selected topology.
- Static host addresses and applicable VIPs belong to `machineCIDR`.
- An explicit rendezvous IP belongs to `machineCIDR`; it is required when no
  master has a static address.
- Host, VIP, DNS, and gateway addresses do not conflict where they share the
  machine network.
- A static address requires a gateway and DNS resolver.
- The gateway belongs to every static host's address prefix.
- Statically addressed hosts have exactly one interface until the schema can
  identify a primary interface.
- `api.<cluster>.<baseDomain>` and `api-int.<cluster>.<baseDomain>` resolve to
  their expected address or load balancer.
- `*.apps.<cluster>.<baseDomain>` resolves to the ingress target.
- Host A/AAAA and PTR records agree when the installation mode requires them.

### 4.3 Topology checks

| Topology | Masters | Workers |
|---|---:|---:|
| SNO | exactly 1 | 0 for initial installation |
| Compact | exactly 3 | 0 or more |
| HA | exactly 3 | 2 or more |

Release-specific installer rules take precedence over this baseline. A changed
upstream requirement must be represented as a compatibility rule and contract
test rather than silently accepted.

SNO uses platform type `none`; the `baremetal` platform is rejected for SNO.

### 4.4 Environment checks

- Output and cache parents exist or can be created and are writable.
- Credential files are regular, readable files with valid content.
- Required external tools exist and report compatible versions.
- Release metadata, installer, and image endpoints are reachable unless
  `--offline` is active.
- Offline mode finds every required item in the verified cache.
- DNS answers come from the configured resolvers, not only the host resolver.
- Host addresses and externally managed VIPs are not already in conflicting use
  when safely testable. When the Fedora helper is configured, API `6443`, MCS
  `22623`, ingress HTTP `80`, and ingress HTTPS `443` must instead be reachable
  on their configured VIPs.

Resource checks report installer-version-appropriate CPU, memory, and disk
requirements. They must not encode one value as valid for every release.

## 5. CLI contract

```text
okub init
okub config validate
okub config render
okub preflight run
okub images sync
okub artifacts build --type iso
okub artifacts build --type pxe
okub artifacts serve
okub lab up|test|down
okub install monitor
okub helper image sync
okub helper render
okub helper up|down [--apply]
okub version
```

Commands added after MVP:

```text
okub tui
```

### 5.1 Global flags

| Flag | Behavior |
|---|---|
| `--config <path>` | Input file; defaults to `./okub.yaml` |
| `--output <path>` | Override configured output directory |
| `--json` | Emit one documented JSON result to stdout |
| `--verbose` | Include diagnostic detail without exposing secrets |
| `--offline` | Forbid network access and require verified cached inputs |
| `--dry-run` | Show intended writes/actions without performing them |

Unsupported flags are rejected. `--dry-run` is exposed only where a command
would write or change state. Prompts are disabled in non-interactive execution;
an action requiring approval fails with instructions unless an explicit command
flag authorizes it.

### 5.2 Streams

- Normal human results use stdout.
- Diagnostics and progress use stderr.
- With `--json`, stdout contains only the JSON result.
- External tool output is captured to a redacted log and summarized to stderr.

### 5.3 Exit codes

| Code | Meaning |
|---:|---|
| 0 | Command completed successfully |
| 1 | Configuration or preflight validation failed |
| 2 | Invalid CLI use or unsupported input version |
| 3 | External tool failed or was incompatible |
| 4 | Network, DNS, or download operation failed |
| 5 | Filesystem, cache, or artifact operation failed |
| 130 | Interrupted by the user |

## 6. Command behavior

### `init`

Writes a minimal commented example after asking for cluster identity, topology,
release, and network values. It never invents credentials or installation disk
paths and does not overwrite an existing file without explicit approval.

### `config validate`

Performs schema and semantic validation without network or external command
checks. It does not write generated installer configuration.

### `config render`

Validates and writes deterministic `install-config.yaml` and
`agent-config.yaml`. It stages all files before publishing any of them.

### `preflight run`

Runs configuration and environment checks and writes the complete report. A
failed check does not prevent independent checks from running.

### `images sync`

Resolves the requested release, downloads and verifies its checksum-pinned HTTP
installer, client, and release-metadata inputs, retains the immutable OCI release-image
pullspec/digest, and atomically inserts the verified result into the cache. A
corrupt cache entry is rejected, not reused. Offline sync does not imply a
disconnected installation: OCI image-layer mirroring remains excluded from MVP.

### `artifacts build --type iso`

Requires successful validation, uses verified inputs, delegates image creation
to `openshift-install`, and publishes an ISO only after the command succeeds and
the output checksum is calculated.

### `artifacts build --type pxe`

Uses the same verified inputs and delegates PXE file creation to
`openshift-install`. It publishes the kernel, initramfs, rootfs reference, and a
checksummed boot manifest as one artifact set.

### `artifacts serve`

Implements the normative [artifact-serving specification](artifact-serving.md).
Serving is separate from artifact generation and never starts implicitly.

### `install monitor`

Runs the appropriate installer wait operation, streams redacted progress, and
preserves the complete redacted log. It does not administer the installed
cluster.

### `helper image sync|render|up|down`

Image sync verifies Fedora's signed checksum metadata and every image byte.
Render writes secret-free NoCloud inputs. Up uses the guarded six-resource
OpenTofu lifecycle and is read-only unless `--apply` is explicit. Down verifies
the state and virsh UUID and only requests bounded graceful shutdown with
`--apply`; it never destroys owned resources. PXE artifact synchronization is
not implemented and readiness therefore fails closed.

## 7. Output contract

```text
build/<cluster>/
├── generated/install-config.yaml
├── generated/agent-config.yaml
├── artifacts/agent.<architecture>.iso
├── artifacts/pxe/kernel
├── artifacts/pxe/initramfs.img
├── artifacts/pxe/rootfs.img
├── artifacts/pxe/agent.ipxe       # when generated by openshift-install
├── artifacts/pxe/manifest.json
├── reports/validation.json
├── reports/preflight.json
├── logs/openshift-install.log
└── run.json
```

`run.json` records:

- schema, Okub, installer, and release versions;
- SHA-256 digest of the normalized input configuration;
- release image digest;
- artifact paths, sizes, and SHA-256 checksums;
- command start/end timestamps and final outcome;
- redacted command arguments and log paths.

Generated YAML has stable key ordering and formatting. Absolute local paths and
secret contents are omitted from machine-readable reports unless required to
identify an output requested by the user.

## 8. Safety and quality requirements

- Rendering identical normalized input with identical tool versions produces
  byte-identical configuration files.
- Existing output is not silently overwritten.
- Multi-file publication is all-or-nothing where the filesystem permits it.
- Cancellation does not leave a completed-looking partial artifact.
- Every download is authenticated by TLS and verified by digest or checksum.
- Secrets are redacted from errors, JSON, logs, and process arguments.
- Read-only commands do not mutate infrastructure.
- JSON field names and check IDs remain backward compatible within an API
  version.

## 9. MVP acceptance

The MVP is complete when:

1. Valid SNO, compact, and HA examples pass schema and semantic validation.
2. Negative fixtures cover conflicting addresses, CIDR overlap, bad topology,
   missing DNS, unavailable releases, corrupt downloads, and failed builds.
3. Okub produces valid installer inputs and Agent ISO/PXE artifacts for the SNO
   scenario from a clean supported environment, and can serve the selected set.
4. Repeating the build reuses verified cache entries and does not corrupt or
   silently replace previous output.
5. The [libvirt validation scenario](libvirt/validation.md) validates the
   bare-metal ISO/PXE boot path and reaches its documented checkpoint.
6. Human and JSON output obey the stream and exit-code contracts.
7. Logs and reports pass tests proving representative secrets are redacted.

See [architecture](../architectures/00-general.md) for implementation
boundaries and [plan](../plan/summary.md) for delivery order.
