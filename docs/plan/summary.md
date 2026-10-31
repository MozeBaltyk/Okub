# Delivery plan summary

Okub is delivered as vertical slices of one bare-metal OKD installation path.
Libvirt validates that path; it is not a separate provider workstream.

## Delivery order

| Phase | Status | Outcome |
|---|---|---|
| [00](phase-00-repository-foundation.md) | complete | Repository and CLI foundation |
| [01](phase-01-configuration-validation.md) | complete | Strict, versioned `okub.yaml` validation |
| [02](phase-02-installer-inputs.md) | complete | Deterministic installer configuration |
| [03](phase-03-preflight.md) | complete | Bare-metal prerequisite report |
| [04](phase-04-release-cache.md) | complete | Verified OKD tools and image cache |
| [05](phase-05-boot-artifacts.md) | PXE gate pending | Automated Agent ISO/PXE contract implemented; real SNO ISO verified |
| [06](phase-06-artifact-serving.md) | core complete, observability/real gate pending | Explicit ephemeral artifact serving |
| [07](phase-07-libvirt-validation.md) | contract complete, KVM gate pending | Disposable validation of the bare-metal path |
| [08](phase-08-mvp-topologies.md) | contract complete, real gate pending | Compact and HA MVP completion |
| [09](phase-09-post-mvp.md) | partial, brought forward | Fedora Cloud 44 helper image, render, and guarded VM lifecycle implemented; other workstreams deferred |

The first release gate is Phase 05: SNO can produce verified boot artifacts.
The MVP gate is Phase 08: ISO/PXE generation and serving are validated and SNO,
compact, and HA configurations satisfy the baseline specification.

## Rules

- Each phase must leave the main branch buildable and testable.
- A later phase may start early only when it does not invent an unfinished
  dependency's contract.
- Status belongs here; target behavior belongs in `docs/specs/`.
- Phase completion requires its acceptance gate, focused tests, and matching
  user-facing documentation.
- Privileged libvirt checks run only on dedicated runners.

## Common quality gate

```bash
go fmt ./...
go vet ./...
go test ./... -count=1
```

See the [repository contract](../00-repo-contract.md),
[baseline specification](../specs/baseline.md), and
[general architecture](../architectures/00-general.md).
