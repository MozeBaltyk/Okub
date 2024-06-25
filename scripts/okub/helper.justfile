set shell := ["bash", "-uc"]

# Ref: https://github.com/ryanhay/ocp4-metal-install?tab=readme-ov-file#download-software

# Install Quay.io Registry for airgap install
QUAY:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install Quay.io Registry for airgap install.\n";

# Install and configure a DNS
DNS:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install and configure a DNS.\n";

# Install and configure a DHCP
DHCP:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install and configure a DHCP.\n";

# Install and configure a HAproxy for Loadbalancing
HAPROXY:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install and configure a HAproxy for Loadbalancing.\n";

# Install and configure a HTTP server for PXE boot
HTTP:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install and configure a HTTP server for PXE boot.\n";
