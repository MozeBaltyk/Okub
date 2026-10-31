# Fedora network-services helper lifecycle specification

Status: implemented baseline; disposable Fedora runtime validation remains required.

## Boundary and ownership

`okub helper up --apply` will create and start an unattended Fedora Cloud 44
Generic x86_64 helper. Planning without `--apply` must not change libvirt, host
networking, storage, or VM state. `okub helper down --apply` will use bounded,
graceful `virsh shutdown` and retain every managed resource and the OpenTofu
state. It must never model down by setting the provider's `running` argument to
false. Destruction remains a separate, saved-plan operation requiring explicit
review and confirmation.

The maintained root module is `lab/fedora-helper`. It uses only local
`qemu:///system`, consumes an existing storage pool, and owns exactly these six
resource addresses:

```text
libvirt_network.wan
libvirt_network.lan
libvirt_volume.base_image
libvirt_volume.root
libvirt_cloudinit_disk.seed
libvirt_domain.helper
```

The base image volume is an immutable provider-managed upload of the verified
host qcow2. The writable root is a qcow2 copy-on-write volume backed by that
managed image. The seed is NoCloud configuration media, not an installer. There
is no installer ISO, CD-ROM, manual console installation, storage-pool resource,
remote libvirt connection, or host networking outside the two networks.

Both networks and the domain have autostart disabled. VNC binds only to
`127.0.0.1`; the serial console is a local libvirt PTY and has no TCP listener.
The WAN is managed NAT with libvirt DHCP and forwarding DNS. The LAN has no
forwarding, libvirt DHCP, or libvirt DNS. Its host bridge uses the final usable
address (`192.168.101.254` for `192.168.101.0/24`), while the helper owns the
first usable address (`192.168.101.1`) and supplies LAN DNS, DHCP, TFTP, HTTP,
and load balancing.

## OpenTofu inputs

Go passes each value as a process argument, never through a shell:

| Variable | Contract |
|---|---|
| `libvirt_uri` | Exactly `qemu:///system` |
| `vm_name` | Explicit unique domain name |
| `storage_pool` | Existing active local libvirt pool name |
| `wan_network_name`, `lan_network_name` | Explicit unique managed network names |
| `wan_bridge_name`, `lan_bridge_name` | Deterministic Linux interface names, at most 15 characters |
| `base_image_volume_name` | Explicit immutable upload name ending `.qcow2` |
| `root_volume_name` | Explicit writable volume name ending `.qcow2` |
| `seed_volume_name` | Explicit NoCloud volume name ending `.iso` |
| `base_image_path` | Absolute existing `.local/cache` path to Fedora Cloud 44 Generic x86_64 qcow2 |
| `base_image_sha256` | SHA-256 verified by Go against Fedora's signed checksum metadata; Terraform rechecks file bytes |
| `cloud_init_user_data_path` | Absolute path to mode-0600, Go-rendered cloud-config |
| `cloud_init_meta_data_path` | Absolute path to mode-0600, Go-rendered NoCloud meta-data |
| `cloud_init_network_config_path` | Absolute path to mode-0600, Go-rendered network-config v2 |
| `wan_cidr` | Dedicated RFC1918 WAN, currently `172.31.255.0/24` |
| `lan_cidr` | Cluster machine CIDR, currently `192.168.101.0/24` |
| `cluster_cidr`, `service_cidr` | Pod and service CIDRs excluded from WAN overlap |
| `wan_excluded_cidrs` | Active host/libvirt CIDRs excluded from WAN overlap |
| `wan_mac`, `lan_mac` | Different explicit locally administered unicast MACs |
| `vcpu`, `memory_mib`, `root_disk_size_gib` | Validated resources; defaults 2, 2048 MiB, and 20 GiB |

Go must reject duplicate names, MACs, bridge names, or managed CIDRs before
OpenTofu. It must validate that the base image is a regular non-symlink file,
matches the expected size and SHA-256, and is the selected Fedora 44 Generic
x86_64 artifact. The requested root size must not be smaller than the image's
virtual size reported by `qemu-img info --output=json`. The image path and generated paths must remain beneath the
repository's `.local` directory. Terraform's hash precondition reduces, but
does not eliminate, a path replacement race; Go must securely open and verify
the cache artifact immediately before planning.

## Cloud-init contract

Go renders complete files and passes only their paths. Terraform does not
template YAML or service configuration, so user-controlled values never cross
a shell interpolation boundary. The seed resource reads the files directly and
marks their contents sensitive for plan display. OpenTofu state and the seed
still contain those bytes: user-data, meta-data, network-config, and service
configuration must contain no passwords, private keys, pull secrets, or other
secrets. Public SSH keys may be included only if remote administration is
explicitly enabled by a later decision.

Before planning, Go parses all three files as YAML and enforces the following
semantic contract internally. Operators may additionally run `cloud-init schema
-c <user-data>` when the executable is installed; it is not a host prerequisite:

- meta-data has unique `instance-id` and the configured local hostname;
- network-config is version 2 and matches interfaces by the exact WAN/LAN MACs;
- WAN uses DHCPv4 and supplies the default route; LAN disables DHCP and default
  routing and assigns `.1/<prefix>` plus the configured API and ingress VIPs;
- user-data enables root/filesystem growth for the larger COW volume, installs
  `dnsmasq`, `haproxy`, and `nginx`, and writes complete owned configuration;
- dnsmasq binds the LAN, provides DNS/DHCP/TFTP, and uses
  `/srv/okub/tftp`; nginx serves `/srv/okub/http`; HAProxy exposes API 6443,
  MCS 22623, ingress HTTP 80, and ingress HTTPS 443 on the configured VIPs;
- `/srv/okub/http` and `/srv/okub/tftp` are created before services start;
- service-specific validation (`dnsmasq --test`, `haproxy -c`, `nginx -t`)
  succeeds before each service is enabled and started; and
- one fixed `set -euo pipefail` setup script installs packages, applies policy,
  validates every configuration, and only then enables services; generated
  configuration data remains separate base64 `write_files` content.

Cloud-init creates empty artifact directories only. It does not claim that PXE
artifacts are present and dnsmasq advertises no boot filename, even when
`boot.method` is `pxe`. Okub's explicit ephemeral Pixiecore command supplies
ProxyDHCP and serves the verified artifact set from the operator host; the
helper remains the site DHCP/DNS/load-balancer simulation and does not receive
artifact copies. A future helper-hosted artifact mode would require a separate
authenticated, digest-verified, atomic synchronization contract. Virtiofs is
not assumed.

## Dependency and plan contract

The first create dependency graph is:

```text
base_image_path -> libvirt_volume.base_image -> libvirt_volume.root
cloud-init files -> libvirt_cloudinit_disk.seed
network variables -> libvirt_network.wan, libvirt_network.lan
root + seed + both networks -> libvirt_domain.helper (created running)
```

Targeting `libvirt_domain.helper` therefore includes all five dependencies. The
domain references every dependency directly and cannot start before they are
created. First or resumed up may contain only the six allowlisted addresses.
Every missing address must have exactly `create`; every present address must be
no-op or omitted; the new domain must have `running=true`. No update, delete, or
replacement is accepted during first-create recovery.

With complete state, up permits no-op or exactly one in-place
`libvirt_domain.helper.running` transition from false to true. Any configuration,
seed, volume, network, domain, or replacement change requires a separately
reviewed workflow. Down requires complete state, verifies the state domain name
and UUID against local virsh, and gracefully shuts down that exact domain. It
does this before any host-overlap or bridge-presence check. Up tolerates an
absent bridge for a state-owned network stopped after reboot, but rejects any
present owned bridge with an unexpected address and any hostile overlap. It
never plans `running=false`, force-stops, destroys, or changes state.

The intended repository-local paths are:

```text
module:       <repo>/lab/fedora-helper
state:        <repo>/.local/fedora-helper/terraform.tfstate
provider data:<repo>/.local/fedora-helper/tofu
plan:         <repo>/.local/fedora-helper/<operation>-<unique-id>.tfplan
operation lock:<repo>/.local/fedora-helper/operation.lock
cloud-init:   <repo>/.local/fedora-helper/cloud-init/{user-data,meta-data,network-config}.yaml
image cache:  <repo>/.local/cache/fedora/44/<verified-fedora-44-image>.qcow2
```

The runtime and provider-data directories are mode 0700; generated inputs,
lock, plans, state, and logs are mode 0600. Symlinked or non-private runtime
objects are rejected, and logs are atomically replaced without following old
paths. One exclusive advisory lock spans prerequisite checks,
init, state and plan inspection, optional apply/shutdown, and cleanup. Each plan
uses a fresh unpredictable name and is applied as saved. Logs are bounded and
redacted; plan/state JSON is inspected in memory and not logged. Environment
must be sanitized and set `TF_IN_AUTOMATION=1` and the explicit `TF_DATA_DIR`.

Go must perform read-only prerequisite checks for `tofu`, `virsh`, local
`qemu:///system`, an active configured pool, cache/image integrity, and YAML
semantics before planning. It must pass the explicit backend path on init and
must not use shell command strings, unqualified local state, `state rm`, `state
mv`, `force-unlock`, `destroy`, or remote libvirt.

## Fedora assumptions and risks

- Fedora Cloud 44 must support NoCloud, network-config v2 through
  NetworkManager, growpart/resizefs, and the selected libvirt machine/CPU.
- Package installation requires working WAN DHCP, DNS, routing, Fedora mirrors,
  and repositories during first boot. A successful domain create does not mean
  cloud-init or services succeeded; readiness must check `cloud-init status
  --wait` and each listener before use.
- Interface matching relies on the explicit MACs, not predicted kernel names.
  Service configs must use the actual stable LAN name selected in network-config.
- Fedora SELinux and firewalld may block nonstandard HAProxy ports, TFTP, or
  nginx content under `/srv/okub`. Go-rendered cloud-init must install and apply
  narrowly scoped persistent policy/firewall rules and validate them on a
  disposable Fedora 44 VM before this is considered production-ready.
- Provider 0.8.3 stores cloud-init content in local state and creates a backing
  chain. Deleting or renaming the base volume outside state can make the root
  unusable. Back up needed artifact data before any separately approved destroy.
- The module and this contract do not migrate or adopt the prior appliance
  state. Its separately reviewed cleanup snapshot and destroy plan remain a
  distinct operation.

## Replacement note

The former appliance implementation and archived appliance files are outside Okub scope. Existing ignored cleanup state and live assets are intentionally untouched and require a separately approved operation.
