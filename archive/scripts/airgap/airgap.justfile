set shell := ["bash", "-uc"]

# MANDATORY
PRODUCT            :=  env_var_or_default('PRODUCT', "okd")
RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest")
OKUB_INSTALL_PATH  :=  env_var_or_default('OKUB_INSTALL_PATH', "${HOME}/" + PRODUCT + "-" + RELEASE_VERSION)

# Download pythons packages defined in ./meta/ee-requirements.txt
airgap_pythons:
    @printf "\e[1;34m[INFO]\e[m ## Make sure Pip package is installed. ##\n"
    @python3 -m venv ../../venv
    @curl --silent --show-error --retry 5 https://bootstrap.pypa.io/get-pip.py | ../../venv/bin/python3
    @printf "\e[1;32m[OK]\e[m Pip installed.\n"
    @printf "\e[1;34m[INFO]\e[m ## Install Pip packages ##\n"
    @mkdir -p {{ OKUB_INSTALL_PATH }}/airgap/pypi
    @../../venv/bin/python3 -m pip download -r ../../meta/ee-requirements.txt -d {{ OKUB_INSTALL_PATH }}/airgap/pypi
    @printf "\e[1;32m[OK]\e[m Pip packages downloaded.\n"

# Download ansible collections defined in ./meta/ee-requirements.yml
airgap_ansible:
    @printf "\e[1;34m[INFO]\e[m ## Airgap Ansible Collections. ##\n"
    @mkdir -p {{ OKUB_INSTALL_PATH }}/airgap/collections
    @ansible-galaxy collection download --ignore-certs -r ../../meta/ee-requirements.yml
    @printf "\e[1;32m[OK]\e[m Ansible collections downloaded.\n"

# Push arkade package in airgap directory
airgap_arkade:
    @printf "\e[1;34m[INFO]\e[m ## Airgap Arkade bin. ##\n"
    @mkdir -p {{ OKUB_INSTALL_PATH }}/airgap/arkade
    @for i in $(cat ../../meta/ee-arkade.txt); do cp ~/.arkade/bin/${i} {{ OKUB_INSTALL_PATH }}/airgap/arkade/. ; done

# Install dependencies from airgap
install_from_airgap_pythons:

# Install dependencies from airgap
install_from_airgap_ansible: