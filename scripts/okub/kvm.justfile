set shell := ["bash", "-uc"]
# Ref: https://github.com/ryanhay/ocp4-metal-install?tab=readme-ov-file#download-software

libvirt_pool_dir          :=  "~/.local/share/libvirt/images/"
fedora_base_image_version :=  "41-1.4"
fedora_base_image_name    :=  "Fedora-Server-KVM-{{ base_image_version }}.x86_64.qcow2"
fedora_base_image_url     :=  "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Server/x86_64/images/{{ base_image_name }}"
fedora_base_image_sha     :=  "1751fcbabcaec44831d60d5439558fac69345c98185fbcaacb4f4c3246d2a404"
hostname                  :=  "bastion"

# Generate a MAC adddress
generate_mac:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Generate a MAC address.\n";
    MAC_ADDR=$(date +%s | md5sum | head -c 6 | sed -e 's/\([0-9A-Fa-f]\{2\}\)/\1:/g' -e 's/\(.*\):$/\1/' | sed -e 's/^/52:54:00:/';echo)
    printf "\e[1;34m[INFO]\e[m Your mac address: ${MAC_ADDR}\n";

# Configure KVM
kvm_config:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Configure KVM.\n";
    sudo systemctl enable --now libvirtd
    sudo systemctl restart libvirtd

# Import qcow2 Fedora image in KVM
kvm_import_fedora_image:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Import Fedora image in KVM.\n";
    curl -L {{ fedora_base_image_url }} -o {{libvirt_pool_dir}}/{{ fedora_base_image_name }};

# Import qcow2 Fedora image in KVM
kvm_import_rhel9_image:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Import RHEL9 image in KVM.\n";

# Create a VM from qcow2 (default latest Fedora)
kvm_create_VM:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Generate a MAC address.\n";
    MAC_ADDR=$(date +%s | md5sum | head -c 6 | sed -e 's/\([0-9A-Fa-f]\{2\}\)/\1:/g' -e 's/\(.*\):$/\1/' | sed -e 's/^/52:54:00:/';echo);
    printf "\e[1;34m[INFO]\e[m Your mac address: ${MAC_ADDR}\n";

    printf "\e[1;34m[INFO]\e[m Create a VM from qcow2 \n";
    cd ../../tests/libvirt && tofu init;
    cd ../../tests/libvirt && tofu plan -out=terraform.tfplan \
      -var "hostname={{ hostname }}" \
      -var "mac_address=${MAC_ADDR}";
    cd ../../tests/libvirt && tofu apply "terraform.tfplan";
    #sleep 60
    #cd ../../tests/libvirt && tofu refresh && tofu output ips;

# Destroy VM from qcow2
kvm_destroy_VM:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Destroy a VM from KVM\n";
    cd ../../tests/libvirt && tofu destroy -auto-approve