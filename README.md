# Okub — bare-metal OKD installation orchestrator

[![CI](https://github.com/mozebaltyk/Okub/actions/workflows/ci.yml/badge.svg)](https://github.com/mozebaltyk/Okub/actions/workflows/ci.yml)
[![Releases](https://img.shields.io/github/release/mozebaltyk/Okub)](https://github.com/mozebaltyk/Okub/releases)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)

> [!WARNING]
> Okub is under active development. Do not use the current development build
> for unattended production changes.

Okub is a local Go CLI that helps operators install
[OKD](https://okd.io/) on bare metal. It validates one declarative cluster
definition, prepares verified inputs for the Agent-based Installer, builds ISO
or PXE artifacts, and monitors installation.

Okub does not replace `openshift-install`. It makes the supported installer path
more reproducible, inspectable, and safe to operate.

## Features

The current implementation supports `x86_64`.

- [x] Strict, versioned YAML cluster definition
- [x] Deterministic `install-config.yaml` and `agent-config.yaml` rendering
- [x] Read-only DNS, network, credential, host, and tool preflight
- [x] Verified OKD release resolution and content-addressed tool cache
- [x] Agent ISO and PXE artifact generation through `openshift-install`
- [x] Checksummed artifacts, redacted logs, and reproducible run metadata
- [x] Installation monitoring through the Agent-based Installer
- [x] Optional Fedora Cloud helper VM with explicit, guarded lifecycle actions
- [x] Built-in ephemeral HTTP/PXE serving with optional Pixiecore
- [x] Compact and HA schema, renderer, and CLI contracts
- [ ] Disposable libvirt end-to-end validation of the physical boot path
- [ ] KVM-backed compact and HA artifact/install acceptance

Artifact generation and serving are implemented and covered by local contract
tests, and a real SNO ISO has been verified. Compact/HA definitions now validate
and render, but their real artifact and installation gates remain pending on a
KVM-capable supported environment. See the [delivery plan](docs/plan/summary.md)
for exact status.

## Usage

Requirements:

- the Go version declared in [`go.mod`](go.mod);
- [`just`](https://github.com/casey/just) for development recipes;
- network access to OKD release metadata and image registries for connected
  builds.

Build the CLI:

```sh
just compile
./bin/okub --help
```

Okub keeps credentials out of `okub.yaml`. Create the files referenced by the
example configuration before running the workflow. For catalog-free testing,
the OKD-documented fake pull secret can be used:

```sh
install -d -m 700 ~/.config/okub
printf '%s\n' '{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}' \
  > ~/.config/okub/pull-secret.json
chmod 600 ~/.config/okub/pull-secret.json
```

Use a real pull secret when authenticated registry or operator catalog access
is required.

Run the installation preparation path:

```sh
# Validate desired state without network checks.
./bin/okub config validate --config examples/sno.yaml

# Inspect the generated installer inputs, then publish them.
./bin/okub config render --config examples/sno.yaml --dry-run
./bin/okub config render --config examples/sno.yaml

# Check the host environment and external prerequisites.
./bin/okub preflight run --config examples/sno.yaml

# Resolve and cache verified release inputs.
./bin/okub images sync --config examples/sno.yaml

# Build one boot format.
./bin/okub artifacts build --type iso --config examples/sno.yaml
# or:
./bin/okub artifacts build --type pxe --config examples/sno.yaml

# After booting the target hosts, wait for installation completion.
./bin/okub install monitor --config examples/sno.yaml
```

Most reports also support `--json`. `--offline` forbids network access where a
workflow can use verified cached inputs. It does not provide disconnected
installation because OCI image layers are not mirrored by Okub.

### Optional network-services helper

The example configuration defines a Fedora VM that can provide DNS, DHCP, TFTP,
HTTP, and load-balancer services in a local lab. Its lifecycle is read-only by
default:

```sh
./bin/okub helper image sync --config examples/sno.yaml
./bin/okub helper render --config examples/sno.yaml
./bin/okub helper up --config examples/sno.yaml
./bin/okub helper up --apply --config examples/sno.yaml
```

The helper does not yet receive PXE artifacts, so it does not claim PXE
readiness. Applying it requires an existing active local libvirt storage pool
and a first boot with repository access.

## How it works

```text
okub.yaml + credential references
              |
              v
strict schema and domain validation
              |
              v
read-only environment preflight ------> DNS / network / tools / credentials
              |
              v
verified release resolver ------------> OKD metadata and content-addressed cache
              |
              v
typed installer input renderer -------> install-config.yaml + agent-config.yaml
              |
              v
openshift-install --------------------> Agent ISO or PXE artifact set
              |
              +-----------------------> operator-managed delivery
              |
              `-----------------------> built-in ephemeral HTTP/PXE serving
                                              |
                                              v
                                      physical OKD hosts
                                              |
                                              v
                                  openshift-install monitoring
```

Each stage consumes verified output from the previous one. Okub stages
multi-file changes before publication, does not silently replace differing
output, and removes private staging data after normal completion.

`openshift-install` remains authoritative for installer formats, host discovery,
and installation progress. Pixiecore transports verified boot artifacts only;
it does not introduce a custom target agent, callback API, per-host SSH
installer, or power-management service.

## Scope

Okub targets:

- physical bare-metal OKD clusters;
- SNO first, followed by compact and HA topologies;
- Agent ISO and PXE boot artifacts;
- existing DHCP or static networking;
- explicit, ephemeral artifact serving rather than permanent provisioning
  infrastructure.

Okub does not manage a cluster after installation, reimplement the installer,
silently change routers or firewalls, or run a permanent DHCP/DNS/PXE service.
Libvirt is validation infrastructure, not a supported deployment target.

## Output

By default, cluster-specific output is published under
`./.local/build/<cluster>` and reusable downloads under `./.local/cache`.
Generated installer inputs, artifacts, checksums, redacted logs, installer
state, and `run.json` remain separate and inspectable. Existing completed output
is replaced only with an explicit `--force` where supported.

Never commit generated artifacts, installer state, cache content, pull secrets,
private keys, or lab state.

## Development

```sh
just fmt
just vet
just test
just compile
```

Real artifact generation may download release-image content and is intentionally
outside the normal unit-test suite. The archived Ansible implementation remains
under [`archive/`](archive/) for reference and is not executed by the Go CLI.

The version can be injected at build time:

```sh
go build -ldflags '-X main.version=v0.1.0' -o bin/okub ./cmd/okub
```

## Documentation

- [Repository contract](docs/00-repo-contract.md)
- [Baseline specification](docs/specs/baseline.md)
- [General architecture](docs/architectures/00-general.md)
- [Bare-metal installation path](docs/architectures/baremetal/01-installation-path.md)
- [Artifact-serving specification](docs/specs/artifact-serving.md)
- [Architectural decisions](docs/decisions/)
- [Delivery plan](docs/plan/summary.md)

Target behavior belongs in `docs/specs/`; implementation status belongs in
`docs/plan/`. The README summarizes both and must not imply that a planned
feature is available.

## Security

Credentials are referenced by path rather than embedded in `okub.yaml`.
Okub redacts credential values from reports and logs, verifies downloaded tools,
uses restrictive permissions for private staging, and requires explicit flags
for infrastructure-changing actions.

Please report suspected vulnerabilities privately rather than opening an issue
containing credentials or installation state.

## License

[Apache License 2.0](LICENSE)
