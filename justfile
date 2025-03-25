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
    @printf "     OKUB_INSTALL_PATH = \$HOME/okd-latest\n"
    @printf "     DOMAIN = example.com\n"
    @printf "     CLUSTER_NAME = okub\n"
    @printf "     MASTERS = 1\n"
    @printf "     WORKERS = 0\n"
    @printf "     DHCP_BOOL = false\n"
    @printf "     LB_BOOL = false\n"
    @printf "     TYPE_OF_INSTALL = \"iso\"\n"
    @printf "     MACHINENETWORK = \"192.168.100.0/24\"\n"
    @printf "\n"
    @printf "VARIABLE NEEDED WHEN INTERNAL REGISTRY\n"
    @printf "     INTERNAL_REGISTRY = <URL:PORT>\n"
    @printf "\n"
    @printf "VARIABLE NEEDED FOR \"just init pxe\"\n"
    @printf "     PXE_SERVER = hostname -i\n"
    @printf "\n"
    @printf "VARIABLE NEEDED WHEN DHCP is FALSE (STATIC NETWORK)\n"
    @printf "     MACADRESS_MASTERS = \"00:ef:44:21:e6:01 00:ef:44:21:e6:02 00:ef:44:21:e6:03\"\n"
    @printf "     MACADRESS_WORKERS = \"00:ef:44:21:e6:04 00:ef:44:21:e6:05\"\n"
    @printf "     INTERFACE = \"ens3\"\n"
    @printf "\n"
    @printf "VARIABLE NEEDED WHEN COREOS SHOULD NOT TAKE ALL THE DISK SPACE\n"
    @printf "     SIZE_PARTITION_VAR = <in MB>\n"
    @printf "DESCRIPTION\n"
    @printf "     This script sets up the environment for OKD/OCP installations.\n"
    @printf "     Customize the behavior by exporting environment variables before\n"
    @printf "     running the 'just' command.\n"
    @printf "\n"

# Install all prerequisites for this projects ["all"|"collections"|"pythons"|"bindeps"|"arkade"].
prerequisites TYPE="all":
    @just -f scripts/prerequis/install.justfile {{ TYPE }}

# ["iso"|"pxe"|""] Generate iso or pxeboot for OKD or OCP install (if args left empty, generate only manifests)
init *OUTCOME:
    @just -f scripts/okub/init.justfile init_install {{OUTCOME}}

# Remove OKD/OCP install
reset:
    @just -f scripts/okub/init.justfile reset_install

# [create|destroy] an helper vm with dhcp/dns/TFPT/Quay.io
helper ACTION="create":
    @just -f scripts/okub/helper.justfile {{ ACTION }}

# [create|destroy] ocp install on KVM
ocp ACTION="create":
    @just -f scripts/okub/ocp.justfile {{ ACTION }}




# Divers pre-checks 
check THAT:
    @just -f scripts/okub/checks.justfile {{ THAT }}

# Watch and Wait for OKD/OCP install to complete at the wanted log LEVEL (default: info) 
wait LEVEL="info":
    @just -f scripts/okub/init.justfile wait {{LEVEL}}

# Add a worker to an existing cluster
add OUTCOME:
    @just -f scripts/okub/add.justfile

# Mirror containers from a namespace ACTION=["create"|"upload"]
mirror_ns NAMESPACE STOREPATH="/tmp":
    @just _oc_mirror_availability
    @just -f scripts/mirror/oc_mirror.justfile from_ns {{NAMESPACE}} {{STOREPATH}}

# Upload in registry an OC archive (give full path to package)
mirror_upload REGISTRY ORG PACKAGE:
    @just -f scripts/mirror/oc_mirror.justfile upload {{REGISTRY}} {{ORG}} {{PACKAGE}}
