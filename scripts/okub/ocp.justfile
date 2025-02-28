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
DHCP_BOOL          :=  env_var_or_default('DHCP_BOOL', "false")
LB_BOOL            :=  env_var_or_default('LB_BOOL', "false")
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.100.0/24")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', '"52:54:00:35:fc:d8", "52:54:00:4f:12:5a", "52:54:00:e2:19:9d"')
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', '"52:54:00:9a:7b:66", "52:54:00:5b:ec:b3"')
# IF VM HELPER is needed
HELPER_HOSTNAME    :=  env_var_or_default('HELPER_HOSTNAME', "helper")
TYPE_OF_INSTALL    :=  env_var_or_default('TYPE_OF_INSTALL', "iso")

# Deploy OCP on libvirtd
create:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Check init folder before OCP install \n";
    if [ ! {{OKUB_INSTALL_PATH}}/.openshift_install_state.json ]; then
        printf "\e[1;31m[NOK]\e[m There is no .openshift_install_state.json file, so probably prokect was not init.";
        exit 1
    fi

    if [[ $(jq -r '.. | objects | select(.Filename? == "tls/root-ca.crt") | .Data' {{OKUB_INSTALL_PATH}}/.openshift_install_state.json  | base64 -d | openssl x509 -noout -startdate | cut -d= -f2 | xargs -I{} date -d {} +%s) -le $(date -d "24 hours" +%s) ]]; 
    then 
        printf "\e[1;32m[OK]\e[m Ready for OCP install \n";
    else
        printf "\e[1;31m[NOK]\e[m Certificate in {{OKUB_INSTALL_PATH}}/.openshift_install_state.json older than 1 days, please reset and init again.\n";
        jq -r '.. | objects | select(.Filename? == "tls/root-ca.crt") | .Data' {{OKUB_INSTALL_PATH}}/.openshift_install_state.json  | base64 -d | openssl x509 -noout -startdate
        exit 1
    fi

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
      -var "okub_install_path={{OKUB_INSTALL_PATH}}" \
      -var "type={{ TYPE_OF_INSTALL }}" \
      ;
    cd ../../libvirt/ocp && tofu apply "terraform.tfplan";

destroy:
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
      -var "okub_install_path={{OKUB_INSTALL_PATH}}" \
      -var "type={{ TYPE_OF_INSTALL }}" \
      ;

haproxy:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m First DNS redirect \n";
    sudo echo -e "[main]\ndns=dnsmasq" | sudo tee /etc/NetworkManager/conf.d/openshift.conf
    sudo echo "server=/okub.example.com/192.168.100.1" | sudo tee /etc/NetworkManager/dnsmasq.d/openshift.conf
    sudo systemctl restart NetworkManager
    sudo resolvectl dns virbr-{{ CLUSTER_NAME }} {{ MACHINENETWORK }} #Get first ip
    sudo resolvectl domain virbr-{{ CLUSTER_NAME }} "~{{ CLUSTER_NAME }}.{{ DOMAIN }}"
    sudo systemctl restart systemd-resolved
    printf "\e[1;34m[INFO]\e[m Create a local HAProxy \n";
    dnf install haproxy
    # Template for haproxy.cfg