set shell := ["bash", "-uc"]
# Ref: https://github.com/ryanhay/ocp4-metal-install?tab=readme-ov-file#download-software

HELPER_HOSTNAME    :=  env_var_or_default('HELPER_HOSTNAME', "helper")
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
# IF LB_BOOL is FALSE
APIVIP             :=  env_var_or_default('APIVIP', "192.168.100.100")
INGRESSVIP         :=  env_var_or_default('INGRESSVIP', "192.168.100.110")
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.100.0/24")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# IF `just init pxe`
PXE_SERVER         :=  env_var_or_default('PXE_SERVER', `hostname -i`)
# IF MASTERS greater or equal to 3 (give one of the master's ip)
RENDEZVOUS_IP      :=  env_var_or_default('RENDEZVOUS_IP', "192.168.100.9")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', "52:54:00:35:fc:d8 52:54:00:4f:12:5a 52:54:00:e2:19:9d")
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', "52:54:00:9a:7b:66 52:54:00:5b:ec:b3")
MACADRESS_BOOTSTRAP:=  env_var_or_default('MACADRESS_BOOTSTRAP', "52:54:00:c5:d3:5f")
IP_MASTERS         :=  env_var_or_default('IP_MASTERS', "192.168.100.10 192.168.100.11 192.168.100.12")
IP_WORKERS         :=  env_var_or_default('IP_WORKERS', "192.168.100.20 192.168.100.21")
INTERFACE          :=  env_var_or_default('INTERFACE', "eno1")
GATEWAY            :=  env_var_or_default('GATEWAY', "192.168.100.1")
DNS_SERVER         :=  env_var_or_default('DNS_SERVER', "192.168.100.3")
NETMASK            :=  env_var_or_default('NETMASK', "255.255.255.0")


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
create:
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
      -var "bootstrap_mac_address={{ MACADRESS_BOOTSTRAP }}" \
      ;
    cd ../../libvirt/helper && tofu apply "terraform.tfplan";

# Destroy VM from qcow2
destroy:
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
      -var "bootstrap_mac_address={{ MACADRESS_BOOTSTRAP }}" \
      ;

# Deploy SNO on libvirtd
sno_create:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m SNO install \n";
    sudo cp {{OKUB_INSTALL_PATH}}/rhcos-live.iso /var/lib/libvirt/images/rhcos-live-{{PRODUCT}}-{{RELEASE_VERSION}}.iso
    printf "\e[1;34m[INFO]\e[m Create a VM from qcow2 \n";
    cd ../../libvirt/sno && tofu init;
    cd ../../libvirt/sno && tofu plan -out=terraform.tfplan \
      -var "product={{ PRODUCT }}" \
      -var "release_version={{RELEASE_VERSION}}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "network_cidr={{ MACHINENETWORK }}" \
      ;
    cd ../../libvirt/sno && tofu apply "terraform.tfplan";


sno_destroy:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Destroy SNO \n";
    cd ../../libvirt/sno && tofu destroy -auto-approve \
      -var "product={{ PRODUCT }}" \
      -var "release_version={{RELEASE_VERSION}}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "network_cidr={{ MACHINENETWORK }}" \
      ;

# https://fajlinuxblog.medium.com/openshift-running-as-single-node-with-libvirt-kvm-cb615d2c43e6