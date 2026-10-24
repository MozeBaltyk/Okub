# Okub

Okub is a Go CLI that helps operators install **OKD on bare metal**.

It guides the installation path without replacing `openshift-install`:

```text
check prerequisites
  → validate okub.yaml
  → render installer configuration
  → resolve verified OKD tools and images
  → build Agent ISO or PXE artifacts
  → export or explicitly serve artifacts
  → monitor installation
```

[![Releases](https://img.shields.io/github/release/mozebaltyk/Okub)](https://github.com/mozebaltyk/Okub/releases)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)

## Status

Okub is under active development. Strict configuration validation,
deterministic Agent-based Installer input rendering, and read-only bare-metal
preflight are available. Deterministic release resolution and verified input
caching are also available. Agent ISO/PXE generation and installation
monitoring are implemented and covered by local contract tests; the real SNO
artifact gate is still pending. Artifact serving remains planned in the
[delivery plan](docs/plan/summary.md).

Do not use the current development build for unattended production changes.
The archived Ansible implementation remains available under [`archive/`](archive/)
for reference but is not maintained or executed by the Go application.

## Scope

The MVP targets:

- OKD on physical bare-metal hosts;
- `x86_64` SNO, compact, and HA topologies;
- Agent-based Installer configuration;
- existing DHCP/static networking, DNS, VIP, and load-balancer prerequisites;
- Agent ISO and PXE artifact generation;
- optional ephemeral HTTP/PXE artifact serving;
- human-readable and JSON validation reports.

Libvirt is used only as disposable test infrastructure to validate the same
artifacts and network path intended for physical hosts. It is not an Okub
deployment target.

Okub does not provide permanent DNS/DHCP/PXE services, reimplement
`openshift-install`, or manage a cluster after installation.

## Development

Requirements:

- the Go version declared in [`go.mod`](go.mod);
- [`just`](https://github.com/casey/just) for repository recipes;
- Podman or Docker only when using the optional execution environment;
- OpenTofu/libvirt only for privileged validation-lab scenarios.

Build and check the CLI:

```bash
just compile
just fmt
just vet
just test
./bin/okub --help
./bin/okub version
./bin/okub config validate --config examples/sno.yaml
./bin/okub config validate --config examples/sno.yaml --json
./bin/okub config render --config examples/sno.yaml --dry-run
./bin/okub config render --config examples/sno.yaml
./bin/okub preflight run --config examples/sno.yaml
./bin/okub preflight run --config examples/sno.yaml --json
./bin/okub preflight run --config examples/sno.yaml --offline
./bin/okub images sync --config examples/sno.yaml
./bin/okub images sync --config examples/sno.yaml --json
./bin/okub images sync --config examples/sno.yaml --offline
./bin/okub artifacts build --type iso --config examples/sno.yaml
./bin/okub artifacts build --type pxe --config examples/sno.yaml
./bin/okub install monitor --config examples/sno.yaml
./bin/okub helper image sync --config examples/sno.yaml
./bin/okub helper render --config examples/sno.yaml
./bin/okub helper up --config examples/sno.yaml          # plan only
./bin/okub helper up --apply --config examples/sno.yaml  # explicit mutation
./bin/okub helper down --apply --config examples/sno.yaml
```

For catalog-free testing, the SNO example can use the fake pull secret
recommended by the OKD documentation. Okub keeps credentials out of
`okub.yaml`, so create the referenced file with restrictive permissions:

```bash
install -d -m 700 ~/.config/okub
printf '%s\n' '{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}' \
  > ~/.config/okub/pull-secret.json
chmod 600 ~/.config/okub/pull-secret.json
```

Use a real pull secret instead when operator catalog or authenticated registry
access is required.

Validation is local and read-only: it applies the documented configuration
defaults and checks structure, names, addresses, networks, uniqueness, and
topology. It does not read credential contents, query DNS, or run environmental
preflight checks. The default input is `./okub.yaml`.

Rendering reads the referenced pull-secret and SSH public-key files and writes
`install-config.yaml` and `agent-config.yaml` under the configured
`<outputDirectory>/generated/`. Output defaults to
`./.local/build/<cluster>` and cache data to `./.local/cache`; credential paths
are unchanged. Use `--output` to override that directory and `--dry-run` to
report `create`, `update`, or `unchanged` without writing. A
differing generated directory requires `--force`; identical output succeeds
without a write. Result details support `--json`, while credential contents are
never printed. DHCP configurations must set `network.rendezvousIP`; static
configurations may omit it and use the lexically first master's address.

Preflight queries every configured DNS resolver directly and checks API,
API-int, apps, host, and PTR answers. With the Fedora helper configured, it requires
reachable API `6443`, MCS `22623`, ingress HTTP `80`, and ingress HTTPS `443`
listeners instead of treating those VIP listeners as conflicts. Other
deployments retain address/VIP-port availability checks. Preflight also checks routes, validates credential content, output/cache access and
compatible `openshift-install`/`oc`, probes required HTTPS endpoints, and reports
the OKD 4.20 host resource minimums. It does not render inputs or change host,
network, or cluster state. Offline mode makes no network probes and instead
requires the same verified release cache produced by `images sync`. A complete
cache also removes any PATH-tool or metadata/tool-download endpoint dependency;
connected builds still require the immutable-image registry.

Image sync selects the newest accepted matching tag from OKD's public
`4-scos-stable` release-controller stream, then resolves that exact GitHub
release. The release-controller detail endpoint supplies the immutable image
digest used to replace the matching image tag with a digest pullspec. Image sync
downloads the matching Linux installer, `oc` client, and `release.txt` only when missing, verifies their
declared sizes and SHA-256 digests, and publishes them atomically. The release
manifest also retains the immutable OCI image pullspec and digest.
Blobs live at `<cache>/blobs/sha256/<digest>` and the manifest at
`<cache>/releases/<requested-version>/<architecture>/manifest.json`. Every hit
is re-verified; a complete hit and every offline invocation make no downloads.
Concurrent syncs safely converge on the same content-addressed entries. The OCI
image layers are not cached: disconnected installation remains outside MVP
scope. The tools are securely materialized under
`./.local/tools/<exact-release>/`. Artifact builds use the verified cached installer and pass the immutable
release pullspec through the installer's supported release-image override. They
stage credential-bearing inputs privately, verify the real Agent output names,
and atomically publish checksummed artifacts, redacted logs, installer state,
and `run.json`. Replacing a completed output requires `--force`.

`install monitor` reuses the compatible cached installer and published state,
records a redacted result under `<outputDirectory>/monitor/`, and follows CLI
cancellation. Real artifact generation can download release image content and
is intentionally not part of normal tests.

The optional `helper` block defines an unattended Fedora Cloud 44 Generic
x86_64 VM using an existing active local libvirt storage pool. Its managed NAT
WAN defaults to `172.31.255.0/24`; its isolated LAN is `machineCIDR`. The helper
uses the first usable LAN address (`.1` in the example) for gateway, DNS, DHCP,
TFTP and HTTP. The host bridge reserves the final usable address (`.254`).

Run the lifecycle in this order:

```bash
./bin/okub helper image sync --config examples/sno.yaml
./bin/okub helper render --config examples/sno.yaml
./bin/okub helper up --config examples/sno.yaml          # inspect only
./bin/okub helper up --apply --config examples/sno.yaml  # creates six resources
virsh -c qemu:///system console okub-lab-helper
./bin/okub preflight run --config examples/sno.yaml
```

Image sync downloads the official Fedora 44 release 1.7 image and signed
CHECKSUM into `.local/cache/fedora/44`, verifies the CHECKSUM with the pinned
Fedora 44 key (`36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6`) using
`gpgv`, then verifies every image byte. Render writes
mode-0600, secret-free NoCloud inputs under
`.local/fedora-helper/cloud-init`. First boot requires WAN DHCP, DNS, routing,
Fedora mirrors and package repositories while cloud-init installs dnsmasq,
HAProxy and nginx. Validate SELinux, firewalld, cloud-init completion and all
listeners on a disposable Fedora 44 VM before production use.
Preflight retries are operator-driven: it checks DNS/PTR and all four HAProxy
listeners but cannot execute `cloud-init status --wait` inside the guest or
prove DHCP leases from the host.

`helper up` is plan-only unless `--apply` is present. It permits only resumable
creation of the exact six owned resources, or a complete-state stopped-to-running
transition. `helper down --apply` verifies the state and virsh UUID, requests a
bounded graceful shutdown, and retains networks, volumes, domain and state. It
never destroys or force-stops resources. Destruction remains a separately
reviewed saved-plan operation.

Cloud-init creates empty `/srv/okub/http` and `/srv/okub/tftp` directories.
Artifact synchronization is intentionally not implemented because no secure
transport/authentication contract exists. Consequently PXE preflight fails
closed until a future verified atomic synchronization command exists; no PXE
boot filename is advertised and no PXE readiness is claimed.

The version can be injected at build time:

```bash
go build -ldflags '-X main.version=v0.1.0' -o bin/okub ./cmd/okub
```

## CLI roadmap

The target command surface is:

```text
okub init
okub config validate
okub config render
okub preflight run
okub images sync
okub artifacts build --type iso
okub artifacts build --type pxe
okub artifacts serve
okub install monitor
okub helper image sync
okub helper render
okub helper up|down [--apply]
```

Commands are implemented incrementally; all commands above except `init` and
`artifacts serve` are currently available. Check `okub --help` rather than
assuming another planned command is available.

## Documentation

- [Repository contract](docs/00-repo-contract.md) — repository rules and docs map
- [General architecture](docs/architectures/00-general.md)
- [Bare-metal installation path](docs/architectures/baremetal/01-installation-path.md)
- [Baseline specification](docs/specs/baseline.md)
- [Delivery plan](docs/plan/summary.md)
- [Architectural decisions](docs/decisions/)

Target behavior belongs in `docs/specs/`; implementation status belongs in
`docs/plan/`; operator procedures will be added only after their workflows are
implemented and tested.

## Security

Never commit pull secrets, private keys, installer state, generated cluster
artifacts, cache content, or lab state. Okub configuration references credential
files; reports and logs must redact their values.

## License

[Apache License 2.0](LICENSE)
