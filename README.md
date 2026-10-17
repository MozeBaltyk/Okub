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

Okub is under active development. Strict configuration validation is available;
preflight, rendering, artifact generation, and serving remain planned in the
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
```

Validation is local and read-only: it applies the documented configuration
defaults and checks structure, names, addresses, networks, uniqueness, and
topology. It does not read credential contents, query DNS, or run environmental
preflight checks. The default input is `./okub.yaml`.

The version can be injected at build time:

```bash
go build -ldflags '-X main.version=v0.1.0' -o bin/okub ./cmd/okub
```

## Planned CLI

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
```

Commands are implemented incrementally. Check `okub --help` rather than
assuming a planned command is available.

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
