set shell := ["bash", "-uc"]

# MANDATORY
PRODUCT            :=  env_var_or_default('PRODUCT', "okd")
RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest")
OKUB_INSTALL_PATH  :=  env_var_or_default('OKUB_INSTALL_PATH', "${HOME}/" + PRODUCT + "-" + RELEASE_VERSION)
DOMAIN             :=  env_var_or_default('DOMAIN', 'example.com')
CLUSTER_NAME       :=  env_var_or_default('CLUSTER_NAME', 'okub')
WORKERS            :=  env_var_or_default('WORKERS', "0")
DHCP_BOOL          :=  env_var_or_default('DHCP_BOOL', "false")
LB_BOOL            :=  env_var_or_default('LB_BOOL', "false")
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.100.0/24")

# Add Workers
add_workers:
    #!/usr/bin/env bash

# Add Masters
add_masters:
    #!/usr/bin/env bash
