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
    @printf "DEFAULT VARIABLES\n"
    @printf "     PRODUCT = {{ PRODUCT }}\n"
    @printf "     RELEASE_VERSION = {{RELEASE_VERSION}}\n"
    @printf "     OKUB_INSTALL_PATH = {{ OKUB_INSTALL_PATH }}\n"
    @printf "     DOMAIN = {{ DOMAIN }}\n"
    @printf "     CLUSTER_NAME = {{ CLUSTER_NAME }}\n"
    @printf "     MASTERS = {{ MASTERS }}\n"
    @printf "     WORKERS = {{ WORKERS }}\n"
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
    @just -f scripts/okub/init.justfile update
    @just -f scripts/okub/init.justfile manifest
    @just -f scripts/okub/init.justfile {{OUTCOME}}

# Add a worker to an existing cluster 
add:
    @just -f scripts/okub/add.justfile 