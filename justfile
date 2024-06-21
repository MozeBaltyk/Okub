set shell := ["bash", "-uc"]

# Lists all available commands in the justfile.
_help:
    @printf "\nNAME\n"
    @printf "     Okub - kickstart OKD/OCP\n"
    @printf "\n"
    @printf "SYNOPSIS\n"
    @printf "     EXPORT VAR=<value> \n"
    @printf "     just recipe [arguments] \n"
    @printf "\n"
    @just --list --unsorted
    @printf "\n"
    @printf "MANDATORY VARIABLES\n"
    @printf "     PRODUCT = okd\n"
    @printf "     RELEASE_VERSION = latest\n"
    @printf "     OKUB_INSTALL_PATH = $HOME/okd-latest\n"
    @printf "     DOMAIN = example.com\n"
    @printf "     CLUSTER_NAME = okub\n"
    @printf "     MASTERS = 1\n"
    @printf "     WORKERS = 0\n"
    @printf "     DHCP_BOOL = FALSE\n"
    @printf "\n"
    @printf "VARIABLE NEEDED FOR \"just init pxe\"\n"
    @printf "     PXE_SERVER = hostname -i\n"
    @printf "\n"
    @printf "VARIABLE NEEDED WHEN 3 MASTERS\n"
    @printf "     RENDEZVOUS_IP = hostname -i\n"
    @printf "\n"
    @printf "VARIABLE NEEDED WHEN DHCP is FALSE (STATIC NETWORK)\n"
    @printf "     MACADRESS_MASTERS = \"00:ef:44:21:e6:m1 00:ef:44:21:e6:m2 00:ef:44:21:e6:m3\"\n"
    @printf "     MACADRESS_WORKERS = \"00:ef:44:21:e6:w1 00:ef:44:21:e6:w2\"\n"
    @printf "     IP_MASTERS = \"xx.xx.xx.x1 xx.xx.xx.x2 xx.xx.xx.x3\"\n"
    @printf "     IP_WORKERS = \"xx.xx.xx.x4 xx.xx.xx.x5\"\n"
    @printf "     DNS_SERVER = \"8.8.8.8\"\n"
    @printf "\n"
    @printf "DESCRIPTION\n"
    @printf "     This script sets up the environment for OKD/OCP installations.\n"
    @printf "     Customize the behavior by exporting environment variables before\n"
    @printf "     running the 'just' command.\n"
    @printf "\n"


# Install helping services to make OKD or OCP install possible
helper METHOD:
    @just -f scripts/okub/helper.justfile {{ METHOD }}

# Divers pre-checks 
check THAT:
    @just -f scripts/okub/checks.justfile {{ THAT }}

# Initiate and produce iso for OKD or OCP install ["iso"|"pxe"]
init OUTCOME:
    @just -f scripts/okub/init.justfile binaries
    @just -f scripts/okub/init.justfile keys
    @just -f scripts/okub/init.justfile install-config
    @just -f scripts/okub/init.justfile agent-config
    @just -f scripts/okub/init.justfile update-install-config
    @just -f scripts/okub/init.justfile update-agent-config
    @just -f scripts/okub/init.justfile manifest
    @just -f scripts/okub/init.justfile {{OUTCOME}}

# Add a worker to an existing cluster 
add OUTCOME:
    @just -f scripts/okub/add.justfile 