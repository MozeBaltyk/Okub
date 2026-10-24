# Fedora network-services helper libvirt module

This root module owns a Fedora Cloud 44 helper domain, an uploaded immutable
base image, its writable copy-on-write root, a NoCloud seed, a dedicated NAT
WAN, and an isolated OKD LAN. It consumes an existing storage pool. Networks and
the VM have autostart disabled; VNC listens only on loopback.

Go downloads and verifies the Fedora Cloud 44 Generic x86_64 qcow2 under
`.local/cache`, renders the three cloud-init inputs without shell interpolation,
and passes absolute paths plus explicit names, MAC addresses, bridge names, and
resource sizing. The provider uploads the verified image into the configured
pool and creates the root from that backing volume. There is no installer ISO,
CD-ROM, interactive install, or remote libvirt support.

WAN NAT, DHCP, and DNS are enabled. LAN forwarding, libvirt DHCP, and libvirt
DNS are disabled. The LAN bridge uses the final usable address (`.254` for a
`/24`), while the helper uses `.1` and runs native dnsmasq, HAProxy, and nginx
configured by cloud-init.

The normative ownership, cloud-init, plan, state, and Go integration contracts
are in [`docs/specs/fedora-helper/lifecycle.md`](../../docs/specs/fedora-helper/lifecycle.md).
Do not invoke this module with an implicit backend: doing so could create a
second state file.
