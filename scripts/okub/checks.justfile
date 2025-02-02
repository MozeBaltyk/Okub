set shell := ["bash", "-uc"]

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

# Check DNS
DNS:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Checks DNS:\n";
    dig +noall +answer @{{ DNS_SERVER }} console-openshift-console.apps.{{ CLUSTER_NAME }}.{{ DOMAIN }}
    dig +noall +answer @{{ DNS_SERVER }} api.{{ CLUSTER_NAME }}.{{ DOMAIN }} 
    dig +noall +answer @{{ DNS_SERVER }} api-int.{{ CLUSTER_NAME }}.{{ DOMAIN }} || printf "\e[1;33m[WARNING]\e[m Not mandatory but bootstrap process faster with.\n"

    # if DHCP true then "plateform: none{}" which require DNS/PTR for all masters and workers. 
    if [[ ! {{ LB_BOOL }} == "FALSE" ]]; then
      if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then
        SNO=true;
        dig +noall +answer @{{ DNS_SERVER }} bootstrap.{{ CLUSTER_NAME }}.{{ DOMAIN }}
      fi

      # master dns
      for ((i=0; i<MASTERS; i++)); do 
        INDEX=$(printf "%02d" $((i + 1)))
        dig +noall +answer @{{ DNS_SERVER }} {{CLUSTER_NAME}}-master'${INDEX}'
      done

      # worker dns
      for ((i=0; i<WORKERS; i++)); do 
        INDEX=$(printf "%02d" $((i + 1)))
        dig +noall +answer @{{ DNS_SERVER }} {{CLUSTER_NAME}}-worker'${INDEX}'
      done
    fi

# Check DHCP
DHCP:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Checks DHCP:\n";

# Check LOADBALANCER
LOADBALANCER:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Checks LOADBALANCER:\n";    

# Check FIREWALL
FIREWALL:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Checks FIREWALL:\n";

# Checks all
ALL:
    #!/usr/bin/env bash
    just -f checks.justfile DNS
    just -f checks.justfile DHCP
    just -f checks.justfile LOADBALANCER
    just -f checks.justfile FIREWALL

# Watch and Wait for OKD/OCP install to complete
wait level:
    #!/usr/bin/env bash
    set -e
    if [ -f {{OKUB_INSTALL_PATH}}/saved/agent-install.yaml ]; then
       # Agent based install
      {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} agent wait-for bootstrap-complete --log-level={{level}}
      {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} agent wait-for install-complete --log-level={{level}}
    else
       # SNO and other type of install 
       {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} wait-for bootstrap-complete --log-level={{level}}
       {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} wait-for install-complete --log-level={{level}}
    fi
