# ADR-04: Keep artifact serving explicit and ephemeral

Status: accepted.

## Context

Bare-metal machines may need ISO content over HTTP or PXE discovery/TFTP, while
many sites already operate DHCP, DNS, and firewall infrastructure. An implicit
or permanent service could disrupt that network.

## Options

1. Install and manage permanent provisioning services.
2. Start ephemeral serving automatically during artifact builds.
3. Export artifacts by default and offer a separate ephemeral serve command.

## Outcome

Artifact generation and serving are separate operations. Okub exports verified
files without requiring a server. `okub artifacts serve` starts explicitly,
binds to an operator-selected address/interface, runs in the foreground by
default, and leaves no permanent service or network change.

HTTP is the base content transport. Pixiecore is optional for PXE behavior and
is supported with existing DHCP only after coexistence testing.
