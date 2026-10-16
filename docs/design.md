
# Okub — Product & Technical Specification

## 1. Product Vision

**Okub** is a CLI/TUI tool for preparing, validating, and bootstrapping OpenShift clusters, primarily using the Agent-based Installer on bare metal and libvirt.

**Principle:** One declarative definition, multiple generated artifacts, reproducible deployment.

## 2. Core Features

- **Configuration:** Generate and validate `install-config.yaml` and `agent-config.yaml`.
- **Preflight:** Check DNS (API, API-int, wildcard apps), networking, NTP, VIPs, connectivity and host inventory.
- **Artifacts:** Download/cache release-compatible CoreOS images; generate Agent ISO or PXE assets.
- **pfSense:** Generate DNS, DHCP, VIP, firewall and PXE-related configuration from cluster definitions.
- **Lab:** Provision a pfSense VM and OpenShift nodes on libvirt for integration testing.
- **Observability:** Display validation reports, command logs, generated artifacts and installation progress.
- **Lifecycle:** Prepare → Validate → Build → Boot → Monitor → Diagnose.

## 3. Scope & Boundaries

**In scope (MVP):** Single cluster, bare metal/VM, Agent-based Installer, ISO, deterministic configs, preflight checks, libvirt lab.

**Next:** PXE, pfSense integration, disconnected installations, multi-cluster profiles.

**Out of scope:** Reimplementing `openshift-install`, operating a permanent provisioning platform, ongoing cluster administration.

## 4. Architecture

```text
CLI (Cobra) / TUI (Bubble Tea)
            │
     Application Layer
  workflows / orchestration
            │
       Domain Model
 cluster / hosts / networks
            │
        Interfaces
 installer / images / DNS / PXE
 firewall / virtualization
            │
     Infrastructure Adapters
 openshift-install / HTTP / pfSense
 Pixiecore / libvirt / Terraform
```

**Rule:** CLI and TUI use the same application services. External dependencies remain behind interfaces.

## 5. Technology Decisions

| Technology | Scope |
|---|---|
| Go | Core application, orchestration, validation |
| Cobra + pflag | CLI commands and options |
| Bubble Tea + Lip Gloss | Interactive TUI and progress views |
| YAML v3 + JSON Schema | Declarative config, parsing and validation |
| Go templates | Generated configurations where appropriate |
| os/exec | `openshift-install` and external binaries |
| net, net/netip, miekg/dns | Network models and DNS checks |
| net/http | Image downloads, APIs and artifact serving |
| SQLite (optional) | Local run history; not required for MVP |
| Terraform/OpenTofu | Disposable libvirt test infrastructure |
| Go testing + testcontainers (optional) | Unit/integration testing |
| GoReleaser | Cross-platform binary releases |

**Recommendation:** Go-only application, modular monolith, no daemon or database for the MVP.

## 6. Canonical Configuration

`okub.yaml` is the single source of truth, with clear sections:

- `cluster`: name, base domain, OpenShift version, architecture, topology.
- `network`: machine/service/cluster CIDRs, DNS, gateway, VIPs.
- `hosts`: hostname, role, MAC, IP, interfaces, installation disk.
- `platform`: bare metal, libvirt, optional platform-specific settings.
- `boot`: ISO/PXE, boot endpoints, artifact locations.
- `pfsense`: interfaces, DHCP reservations, DNS records, firewall policies.
- `artifacts`: cache, output paths, checksum policy.

Generated OpenShift/pfSense files must support inspection and diffing; never overwrite manually without confirmation.

## 7. CLI/TUI Experience

```bash
okub init
okub config validate
okub config render
okub preflight run
okub images sync
okub artifacts build --type iso
okub artifacts build --type pxe
okub pfsense render
okub lab up
okub lab test
okub lab down
okub install monitor
okub tui
```

Global flags: `--config`, `--output`, `--offline`, `--dry-run`, `--json`, `--verbose`.

## 8. Project Structure

```text
.
├── cmd/okub/                 # Entrypoint
├── internal/
│   ├── domain/               # Cluster/network/host models
│   ├── application/          # Use cases and workflows
│   ├── ports/                # External dependency interfaces
│   ├── adapters/
│   │   ├── openshift/
│   │   ├── coreos/
│   │   ├── dns/
│   │   ├── pfsense/
│   │   ├── pxe/
│   │   └── libvirt/
│   ├── cli/
│   └── tui/
├── schemas/                  # Okub config schemas
├── templates/                # Generated config templates
├── examples/                 # SNO, compact, HA
├── docs/
│   ├── 00-repo-contract.md
│   ├── architecture/
│   ├── decisions/            # ADRs
│   ├── plan/
│   ├── procedures/
│   ├── specs/
│   └── troubleshooting/
├── tests/
│   ├── unit/
│   ├── integration/
│   ├── e2e/
│   ├── fixtures/
│   └── scenarios/
├── lab/
│   └── terraform/libvirt/
├── scripts/                  # EE lifecycle + shared tooling helpers
├── .github/workflows/
├── justfile                  # Build/test/EE entrypoints
├── Containerfile             # Dev execution-environment image
├── helm/                     # okub-ee chart (podman play kube / K8s)
└── go.mod
```

### `scripts/`

Shell glue invoked by the justfile (and CI), deliberately kept out of the
compiled binary. `ee/` builds, pushes and runs the okub-ee execution
environment (`manage.sh`); `utility/` holds shared host/tooling checks
(`check-tools.sh`). Business logic lives in the Go CLI under `internal/` —
scripts only orchestrate.

## 9. Third-Party Integrations

- **openshift-install:** Authoritative generator of Agent ISO/PXE assets; pin versions and capture logs.
- **RHCOS/SCOS:** Handle release-specific image sources and compatibility; don't assume Fedora CoreOS images are interchangeable.
- **[Pixiecore](https://github.com/danderson/netboot/tree/main/pixiecore):** Strong candidate for optional ephemeral PXE serving, including ProxyDHCP/TFTP/HTTP and MAC-aware boot APIs. Test coexistence with pfSense DHCP.
- **[Nixie](https://github.com/khuedoan/nixie):** Architectural inspiration for stateless, ephemeral provisioning; not a direct dependency because it targets NixOS.
- **pfSense:** Start with config generation and validation; treat applying changes as a separate, opt-in capability.
- **libvirt/OpenTofu:** Disposable test environments, isolated networks and snapshots.

## 10. Testing Strategy

- Unit: model validation, CIDRs, DNS records, renderers and invalid inputs.
- Contract: supported installer versions, CLI outputs, schema compatibility.
- Integration: DNS resolution, artifact checksums, PXE HTTP endpoints.
- E2E: libvirt pfSense + SNO/compact/HA scenarios as resources permit.
- Negative: conflicting IPs, missing DNS, unavailable images, failed boots.
- CI: fast unit tests on each PR; privileged/libvirt tests on dedicated runners.

## 11. Delivery Roadmap

1. **M0 — Foundation:** Repository contract, ADRs, config schema, CLI skeleton.
2. **M1 — Prepare:** Config generation, validation, preflight and OpenShift integration.
3. **M2 — Build:** Version-aware image cache, Agent ISO and PXE artifacts.
4. **M3 — Network:** pfSense renderer, diagnostics and libvirt tests.
5. **M4 — UX:** TUI, progress monitoring, reports and troubleshooting.
6. **M5 — Advanced:** Ephemeral PXE service, disconnected and multi-cluster support.

## 12. Architectural Principles

- Prefer orchestration over reimplementation.
- Separate desired state, generated artifacts and runtime state.
- Deterministic rendering, idempotent operations, atomic writes.
- Never silently mutate physical infrastructure or firewall configuration.
- Pin binaries/images, verify checksums, redact secrets.
- Make every workflow scriptable, testable and usable without the TUI.
- **Keep Pixiecore optional; keep `openshift-install` authoritative.**
