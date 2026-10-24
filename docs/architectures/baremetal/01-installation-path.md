# Bare-metal installation path

This view describes how the general Okub components collaborate around a
physical OKD installation. Observable requirements remain in
[`specs/baseline.md`](../../specs/baseline.md).

```text
operator + okub.yaml
        |
        v
config/schema/domain validation
        |
        v
environment preflight ──> DNS / network / host / credential checks
        |
        v
release resolver ───────> verified installer and image cache
        |
        v
typed renderers ────────> install-config.yaml + agent-config.yaml
        |
        v
openshift-install ──────> Agent ISO or PXE artifact set
        |
        +───────────────> export to operator-managed infrastructure
        |
        └───────────────> explicit ephemeral HTTP/PXE serving
                                  |
                                  v
                           physical OKD hosts
```

Okub separates checking, rendering, building, and serving so each stage is
inspectable and scriptable. A later stage consumes verified output from the
previous stage; it does not silently repair or mutate prerequisites.

Physical DNS, DHCP, routing, load balancing, firewalling, and machine firmware
remain site-owned. Preflight observes them. The optional Fedora helper supplies
configuration, but applying it remains a separate explicit action.

`openshift-install` owns installer semantics and Agent asset formats. Okub owns
input validation, safe process orchestration, checksums, publication, reports,
and redacted diagnostics.

Artifact export requires no Okub network service. When serving is selected,
Okub uses the same published artifact manifest and starts only the listeners
required by the chosen mode. See the
[artifact-serving spec](../../specs/artifact-serving.md).
