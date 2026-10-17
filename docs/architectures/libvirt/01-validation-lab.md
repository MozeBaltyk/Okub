# Libvirt validation-lab architecture

Libvirt is an end-to-end test adapter around the bare-metal installation path,
not a user-facing provider.

```text
normal okub.yaml + published artifacts
                 |
                 v
       Okub application workflows
          |                 |
          |                 └── normal HTTP/PXE server adapter
          v
OpenTofu/libvirt lab adapter
          |
          +── isolated test network and service simulation
          └── VM firmware/disk/NIC representing a physical host
```

The lab consumes the same cluster definition, installer inputs, manifests, and
servers used for physical hosts. Lab-only details such as VM memory, pool, and
network ownership stay in the test harness rather than extending `okub.yaml`
with a libvirt platform.

Every resource receives a unique run ownership identifier. Teardown discovers
resources by that identifier and cannot select unrelated guests or networks by
name prefix alone. Failed runs preserve redacted diagnostics while allowing a
later explicit cleanup.

OpenTofu manages disposable lab infrastructure; it does not become part of the
bare-metal runtime. Privileged scenarios run only on dedicated CI runners.

Normative behavior is in the
[libvirt validation spec](../../specs/libvirt/validation.md).
