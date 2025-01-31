set shell := ["bash", "-uc"]
# Ref: https://github.com/ryanhay/ocp4-metal-install?tab=readme-ov-file#download-software

# MANDATORY
PRODUCT            :=  env_var_or_default('PRODUCT', "okd")
RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest")
OKUB_INSTALL_PATH  :=  env_var_or_default('OKUB_INSTALL_PATH', "${HOME}/" + PRODUCT + "-" + RELEASE_VERSION)
DOMAIN             :=  env_var_or_default('DOMAIN', 'example.com')
CLUSTER_NAME       :=  env_var_or_default('CLUSTER_NAME', 'okub')
MASTERS            :=  env_var_or_default('MASTERS', "1")
WORKERS            :=  env_var_or_default('WORKERS', "0")
DHCP_BOOL          :=  env_var_or_default('DHCP_BOOL', "FALSE")
LB_BOOL            :=  env_var_or_default('LB_BOOL', "FALSE")
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.100.0/24")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', '"52:54:00:35:fc:d8", "52:54:00:4f:12:5a", "52:54:00:e2:19:9d"')
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', '"52:54:00:9a:7b:66", "52:54:00:5b:ec:b3"')
# IF VM HELPER is needed
HELPER_HOSTNAME    :=  env_var_or_default('HELPER_HOSTNAME', "helper")

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

# Create a VM from qcow2 (default latest Fedora)
helper_create:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Generate a MAC address.\n";
    HELPER_MAC_ADDR=$(date +%s | md5sum | head -c 6 | sed -e 's/\([0-9A-Fa-f]\{2\}\)/\1:/g' -e 's/\(.*\):$/\1/' | sed -e 's/^/52:54:00:/';echo);
    printf "\e[1;34m[INFO]\e[m Your mac address: ${HELPER_MAC_ADDR}\n";

    printf "\e[1;34m[INFO]\e[m Create a VM from qcow2 \n";
    cd ../../libvirt/helper && tofu init;
    cd ../../libvirt/helper && tofu plan -out=terraform.tfplan \
      -var "hostname={{ HELPER_HOSTNAME }}" \
      -var "helper_mac_address=${HELPER_MAC_ADDR}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      -var "masters_mac_addresses={{ MACADRESS_MASTERS }}" \
      -var "workers_mac_addresses={{ MACADRESS_WORKERS }}" \
      ;
    cd ../../libvirt/helper && tofu apply "terraform.tfplan";

# Destroy VM from qcow2
helper_destroy:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Destroy a VM from KVM\n";
    cd ../../libvirt/helper && tofu destroy -auto-approve
      -var "hostname={{ HELPER_HOSTNAME }}" \
      -var "helper_mac_address=${HELPER_MAC_ADDR}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      -var "masters_mac_addresses={{ MACADRESS_MASTERS }}" \
      -var "workers_mac_addresses={{ MACADRESS_WORKERS }}" \
      ;

# Deploy OCP on libvirtd
ocp_create:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m OCP install \n";
    sudo cp {{OKUB_INSTALL_PATH}}/cache/rhcos-master.iso /var/lib/libvirt/images/rhcos-master-{{PRODUCT}}-{{RELEASE_VERSION}}.iso
    sudo cp {{OKUB_INSTALL_PATH}}/cache/rhcos-worker.iso /var/lib/libvirt/images/rhcos-worker-{{PRODUCT}}-{{RELEASE_VERSION}}.iso
    printf "\e[1;34m[INFO]\e[m Create a VM from qcow2 \n";

    cd ../../libvirt/ocp && tofu init;
    cd ../../libvirt/ocp && tofu plan -out=terraform.tfplan \
      -var "product={{ PRODUCT }}" \
      -var "release_version={{RELEASE_VERSION}}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "network_cidr={{ MACHINENETWORK }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      -var 'masters_mac_addresses=[{{ MACADRESS_MASTERS }}]' \
      -var 'workers_mac_addresses=[{{ MACADRESS_WORKERS }}]' \
      -var "dhcp_bool={{ DHCP_BOOL }}" \
      -var "lb_bool={{ LB_BOOL }}" \
      ;
    cd ../../libvirt/ocp && tofu apply "terraform.tfplan";


ocp_destroy:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Destroy OCP \n";
    cd ../../libvirt/ocp && tofu destroy -auto-approve \
      -var "product={{ PRODUCT }}" \
      -var "release_version={{RELEASE_VERSION}}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "network_cidr={{ MACHINENETWORK }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      -var 'masters_mac_addresses=[{{ MACADRESS_MASTERS }}]' \
      -var 'workers_mac_addresses=[{{ MACADRESS_WORKERS }}]' \
      -var "dhcp_bool={{ DHCP_BOOL }}" \
      -var "lb_bool={{ LB_BOOL }}" \
      ;
