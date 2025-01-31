set shell := ["bash", "-uc"]

# MANDATORY
PRODUCT            :=  env_var_or_default('PRODUCT', "okd")
RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest")
OKUB_INSTALL_PATH  :=  env_var_or_default('OKUB_INSTALL_PATH', "${HOME}/" + PRODUCT + "-" + RELEASE_VERSION)
DOMAIN             :=  env_var_or_default('DOMAIN', 'example.com')
CLUSTER_NAME       :=  env_var_or_default('CLUSTER_NAME', 'okub')
MASTERS            :=  env_var_or_default('MASTERS', "1")
WORKERS            :=  env_var_or_default('WORKERS', "0")
DHCP_BOOL          :=  env_var_or_default('DHCP_BOOL', "false")
LB_BOOL            :=  env_var_or_default('LB_BOOL', "false")
HELPER_BOOL        :=  env_var_or_default('HELPER_BOOL', "false")
# IF LB_BOOL is FALSE
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.100.0/24")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# IF `just init pxe`
PXE_SERVER         :=  env_var_or_default('PXE_SERVER', `hostname -i`)
# IF MASTERS greater or equal to 3 (give one of the master's ip)
RENDEZVOUS_IP      :=  env_var_or_default('RENDEZVOUS_IP', "192.168.100.9")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', '"52:54:00:35:fc:d8", "52:54:00:4f:12:5a", "52:54:00:e2:19:9d"')
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', '"52:54:00:9a:7b:66", "52:54:00:5b:ec:b3"')
INTERFACE          :=  env_var_or_default('INTERFACE', "ens3")

# Initialize OCP install
ocp_init_create *outcome:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Initialize OCP isntall.\n";
    cd ../../libvirt/init && tofu init;
    cd ../../libvirt/init && tofu plan -out=terraform.tfplan \
      -var "product={{ PRODUCT }}" \
      -var "release_version={{RELEASE_VERSION}}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "network_cidr={{ MACHINENETWORK }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      -var 'masters_mac_addresses=[{{ MACADRESS_MASTERS }}]' \
      -var 'workers_mac_addresses=[{{ MACADRESS_WORKERS }}]' \
      -var "internal_registry={{ INTERNAL_REGISTRY }}" \
      -var "okub_install_path={{ OKUB_INSTALL_PATH }}" \
      -var "network_interface={{ INTERFACE }}" \
      -var "dhcp_bool={{ DHCP_BOOL }}" \
      -var "lb_bool={{ LB_BOOL }}" \
      -var "helper_bool={{ HELPER_BOOL }}" \
      -var "option={{ outcome }}" \
      ;
    cd ../../libvirt/init && tofu apply "terraform.tfplan";

# Initialize OCP install
ocp_init_destroy:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Destroy OCP isntall.\n";
    cd ../../libvirt/init && tofu destroy -auto-approve \
      -var "product={{ PRODUCT }}" \
      -var "release_version={{RELEASE_VERSION}}" \
      -var "clusterid={{ CLUSTER_NAME }}" \
      -var "domain={{ DOMAIN }}" \
      -var "network_cidr={{ MACHINENETWORK }}" \
      -var "masters_number={{ MASTERS }}" \
      -var "workers_number={{ WORKERS }}" \
      -var 'masters_mac_addresses=[{{ MACADRESS_MASTERS }}]' \
      -var 'workers_mac_addresses=[{{ MACADRESS_WORKERS }}]' \
      -var "internal_registry={{ INTERNAL_REGISTRY }}" \
      -var "okub_install_path={{ OKUB_INSTALL_PATH }}" \
      -var "network_interface={{ INTERFACE }}" \
      -var "dhcp_bool={{ DHCP_BOOL }}" \
      -var "lb_bool={{ LB_BOOL }}" \
      -var "helper_bool={{ HELPER_BOOL }}" \
    ;
