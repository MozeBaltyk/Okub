# Repository contract

What every change to Okub must preserve. This is the repository governance and
documentation map. The normative product behavior lives in `docs/specs/`; when
general prose disagrees with a specification, the specification wins on what
Okub must do.

## Product boundary

Okub helps operators install OKD on bare metal. Its supported path is:

```text
check prerequisites
  ↓
load and validate okub.yaml
  ↓
render and validate installer configuration
  ↓
resolve verified OKD tools and images
  ↓
produce ISO or PXE artifacts
  ↓
export or explicitly serve those artifacts
  ↓
monitor installation
```

Libvirt exists only to test and validate that bare-metal path. It is not a
deployment provider, a user-facing platform, or a second product workflow.

Okub orchestrates `openshift-install`; it does not reimplement the installer,
run a permanent provisioning platform, or perform day-two cluster management.

## Toolchain

Go is the application language. The repository currently declares Go 1.27 in
`go.mod`; changing that version requires CI and execution-environment updates in
the same change. `just` is the development entrypoint, while the compiled
`okub` binary is the user-facing interface.

External tools are adapters, not sources of business logic:

- `openshift-install` is authoritative for Agent ISO and PXE generation;
- OpenTofu/libvirt provides disposable validation infrastructure only;
- Pixiecore may provide optional ephemeral PXE discovery/TFTP;
- Fedora helper support separates verified image sync, deterministic cloud-init rendering, and explicit lifecycle apply; applying
  changes is always separate and explicit.

Tool and image versions are pinned or resolved reproducibly, checksums/digests
are verified, and versions are written to run metadata. Credentials come from
referenced files or the environment and are never committed.

## Dependency direction

```text
CLI / TUI
    ↓
application workflows
    ↓
domain model and rules
    ↓
ports
    ↓
infrastructure adapters
```

Dependencies never point upward. The domain must not import Cobra, Bubble Tea,
process execution, network clients, or infrastructure adapters. CLI and TUI use
the same application workflows. Shell scripts and Just recipes remain thin and
must not contain product rules.

## Repository layout

```text
cmd/okub/                     process entrypoint and dependency wiring
internal/domain/              models, invariants, pure validation
internal/application/         workflows and application result types
internal/ports/               narrow interfaces required by workflows
internal/adapters/            filesystem, installer, DNS, HTTP, PXE, lab
internal/cli/                 Cobra commands and presentation
internal/tui/                 optional Bubble Tea presentation
docs/                         repository contract and categorized documentation
schemas/                      versioned Okub configuration schemas
templates/                    templates only where typed output is unsuitable
examples/                     complete supported cluster definitions
lab/terraform/libvirt/        disposable validation infrastructure
tests/                        integration, end-to-end, fixtures, scenarios
scripts/                      execution-environment and tooling glue only
```

Do not add interfaces, packages, configuration options, or extension systems
before a supported workflow needs them. Shared behavior belongs in domain or
application code, never copied between adapters.

## Configuration contract

`okub.yaml` is the desired-state source of truth. Its API version, strict field
contract, defaults, and invariants are defined in
[`specs/baseline.md`](specs/baseline.md). Schema and Go type changes must land
together with examples, negative fixtures, renderers, and compatibility tests.

Unknown fields are errors. Okub does not guess networks, host addresses,
installation disks, release versions, or credentials. Secret values are
referenced, never embedded in reports or run metadata.

## State and artifacts

Keep these states separate:

1. desired state: user-owned configuration and credential references;
2. generated state: inspectable installer inputs and boot artifacts;
3. runtime state: reports, redacted logs, progress, and run metadata;
4. cache state: reusable, content-addressed, verified downloads.

Generated files are deterministic and staged before publication. Never silently
overwrite user files, publish an incomplete artifact, or treat an unverified
cache entry as valid. Infrastructure-changing commands are explicit; dry-run
commands perform no action.

Artifact serving is opt-in and ephemeral. It binds only to an explicitly
selected address/interface, runs in the foreground by default, and installs no
permanent service. It must not alter an existing DHCP, DNS, firewall, or router
configuration as a side effect.

## Testing and validation

Every implementation change runs the smallest relevant checks. The baseline is:

```bash
go fmt ./...
go vet ./...
go test ./... -count=1
```

Additional requirements:

- schema changes validate every example and negative fixture;
- CLI changes test help, exit codes, stdout/stderr, and JSON contracts;
- renderers use deterministic golden files;
- process, download, and serving changes test cancellation and failure cleanup;
- secret-handling changes include adversarial redaction tests;
- libvirt end-to-end tests run only on dedicated privileged runners and clean up
  only resources carrying explicit ownership metadata.

Tests must not depend on public endpoints when a local fixture server is enough.
No paid, destructive, physical-host, router, or firewall operation runs without
explicit user approval.

## Workflow

- Do not work directly on `main`; use a feature branch.
- Work in small, reviewable vertical slices and keep the main branch buildable.
- A behavior change updates its spec, implementation, tests, examples, and help
  in the same change.
- A configuration field change updates Go types, schema, examples, fixtures,
  rendering, and validation together.
- Keep [`plan/summary.md`](plan/summary.md) current. Detailed implementation
  work belongs in one phase file, not in ad hoc root notes.
- Do not commit generated cluster artifacts, pull secrets, private keys, cache
  content, installer state, or libvirt state.
- Do not commit automatically; commits are explicit user actions.

## Documentation map

Keep this basic structure, including empty category directories:

```text
docs/
├── 00-repo-contract.md
├── architectures/
├── decisions/
├── plan/
├── procedures/
├── specs/
└── troubleshooting/
```

`00-repo-contract.md` is mandatory and is the only content file allowed directly
under `docs/`. Never move it into a category directory.

| Path | Holds |
|---|---|
| `00-repo-contract.md` | Repository rules, boundaries, workflow, and this map |
| `plan/` | `summary.md` index plus one file per implementation phase |
| `specs/` | Normative target behavior: shared baseline plus workstream specs |
| `architectures/` | How the system is built; general architecture plus scoped views |
| `decisions/` | Stable numbered ADRs recording context, options, and outcome |
| `procedures/` | Tested operator how-tos for implemented workflows |
| `troubleshooting/` | Symptom → cause → diagnosis → fix notes from real failures |

Documentation rules:

- One topic, one authoritative place. Link instead of copying a contract.
- Use the exact category names above, including `plan/` rather than `plans/`.
- A cross-file workstream gets one spec containing scope, interfaces,
  lifecycle, constraints, and acceptance criteria.
- Specs describe target behavior; architectures explain implementation design;
  plans track work; procedures tell operators how; ADRs preserve lasting
  rationale; troubleshooting documents symptoms, causes, and fixes.
- User-facing usage, current status, roadmap, and limits stay in `README.md`.
- Use stable `ADR-NN-concise-subject.md` names for decisions.
- Empty documentation directories contain `.gitkeep`; otherwise create a
  document only when it has content.
- Temporary analysis may exist while work is active, but must be dispatched to
  its lasting location or deleted before the workstream closes.

## Planning process

Implementation planning lives in `docs/plan/`:

1. Add or update the workstream in `summary.md`.
2. Keep goal, status, scope, dependencies, and acceptance gate in its phase
   file.
3. During implementation, move durable outcomes to the correct destination:
   target behavior to `specs/`, design to `architectures/`, operational steps to
   `procedures/`, and lasting rationale to `decisions/`.
4. Mark the phase complete only after implementation, focused validation, and
   user-facing documentation agree.
5. Delete temporary work logs; do not leave orphan documents under `docs/`.
