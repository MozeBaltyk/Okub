set shell := ["bash", "-uc"]

# Ref: https://github.com/ryanhay/ocp4-metal-install?tab=readme-ov-file#download-software

# Install Install and configure KVM
kvm:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install and configure KVM.\n";

# Create a VM from qcow (default latest Fedora)
create_VM:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Create a VM from qcow (default latest Fedora)\n";
