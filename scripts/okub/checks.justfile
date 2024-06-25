set shell := ["bash", "-uc"]

# MANDATORY
PRODUCT            :=  env_var_or_default('PRODUCT', "okd")
RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest")
OKUB_INSTALL_PATH  :=  env_var_or_default('OKUB_INSTALL_PATH', "${HOME}/" + PRODUCT + "-" + RELEASE_VERSION)
DOMAIN             :=  env_var_or_default('DOMAIN', 'example.com')
CLUSTER_NAME       :=  env_var_or_default('CLUSTER_NAME', 'okub')
MASTERS            :=  env_var_or_default('MASTERS', "1")
WORKERS            :=  env_var_or_default('WORKERS', "0")
PLATEFORM          :=  env_var_or_default('PLATEFORM', "none")
DHCP_BOOL          :=  env_var_or_default('DHCP', "FALSE")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# IF `just init pxe`
PXE_SERVER         :=  env_var_or_default('PXE_SERVER', `hostname -i`)
# IF MASTERS greater or equal to 3 (give one of the master's ip)
RENDEZVOUS_IP      :=  env_var_or_default('RENDEZVOUS_IP', "192.168.111.11")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', "00:ef:44:21:e6:m1 00:ef:44:21:e6:m2 00:ef:44:21:e6:m3")
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', "00:ef:44:21:e6:w1 00:ef:44:21:e6:w2")
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
    dig +noall +answer @{{ DNS_SERVER }} api-int.{{ CLUSTER_NAME }}.{{ DOMAIN }}
    if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then 
      dig +noall +answer @{{ DNS_SERVER }} bootstrap.{{ CLUSTER_NAME }}.{{ DOMAIN }}
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