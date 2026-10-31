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

## Ephemeral PXE runtime

```text
operator
   |
   | okub artifacts serve --mode pxe
   v
verify manifest, files, network selection, ports, and privileges
   |
   v
bind ProxyDHCP + TFTP + HTTP as one supervised serving unit
   |
   v
report ready only after every required listener is accepting traffic
   |
   +<--- firmware PXE request
   |       normalize architecture and MAC
   |       reject unsupported or unknown clients
   |
   +---> iPXE program and host boot specification
   +---> verified kernel, initramfs, and rootfs reference
   |
   v
Agent-based Installer takes installation authority
   |
   v
openshift-install monitor
```

The bind address controls local listeners; the advertised address must be
reachable by target hosts and is used in generated boot URLs. Boot requests may
be repeated and do not create installation state. Okub supervises transport and
captures redacted diagnostics, while `openshift-install` owns host discovery,
installation progress, and completion. This boundary is recorded in
[ADR-05](../../decisions/ADR-05-boot-transport-boundary.md).
