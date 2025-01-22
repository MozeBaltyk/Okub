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
DHCP_BOOL          :=  env_var_or_default('DHCP', "FALSE")
LB_BOOL            :=  env_var_or_default('LB', "FALSE")
# IF LB_BOOL is FALSE
APIVIP             :=  env_var_or_default('APIVIP', "192.168.10.10")
INGRESSVIP         :=  env_var_or_default('INGRESSVIP', "192.168.10.11")
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.10.0/24")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# IF `just init pxe`
PXE_SERVER         :=  env_var_or_default('PXE_SERVER', `hostname -i`)
# IF MASTERS greater or equal to 3 (give one of the master's ip)
RENDEZVOUS_IP      :=  env_var_or_default('RENDEZVOUS_IP', "192.168.111.11")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', "00:14:22:01:23:45 00:25:96:12:34:56 00:50:56:C0:00:08")
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', "00:0C:29:4F:8E:35 00:1A:4B:16:01:59")
IP_MASTERS         :=  env_var_or_default('IP_MASTERS', "192.168.111.11 192.168.111.12 192.168.111.13")
IP_WORKERS         :=  env_var_or_default('IP_WORKERS', "192.168.111.14 192.168.111.15")
INTERFACE          :=  env_var_or_default('INTERFACE', "eno1")
GATEWAY            :=  env_var_or_default('GATEWAY', "192.168.111.253")
DNS_SERVER         :=  env_var_or_default('DNS_SERVER', "192.168.111.1")


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
    MAC_ADDR=$(date +%s | md5sum | head -c 6 | sed -e 's/\([0-9A-Fa-f]\{2\}\)/\1:/g' -e 's/\(.*\):$/\1/' | sed -e 's/^/52:54:00:/';echo);
    printf "\e[1;34m[INFO]\e[m Your mac address: ${MAC_ADDR}\n";

    printf "\e[1;34m[INFO]\e[m Create a VM from qcow2 \n";
    cd ../../tests/libvirt && tofu init;
    cd ../../tests/libvirt && tofu plan -out=terraform.tfplan \
      -var "hostname={{ HELPER_HOSTNAME }}" \
      -var "mac_address=${MAC_ADDR}" \
      -var "clusterid= {{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      ;
    cd ../../tests/libvirt && tofu apply "terraform.tfplan";
    #sleep 60
    #cd ../../tests/libvirt && tofu refresh && tofu output ips;

# Destroy VM from qcow2
destroy:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Destroy a VM from KVM\n";
    cd ../../tests/libvirt && tofu destroy -auto-approve