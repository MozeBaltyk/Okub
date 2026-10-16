set shell := ["bash", "-uc"]

# Install all prerequisites for this projects.
all: pythons bindeps collections arkade

# Get package manager
_get_pkg_manager:
    #!/usr/bin/env bash
    if [ -f /etc/redhat-release ]; then
        export PKG_MANAGER="yum"
    elif [ -f /etc/debian_version ]; then
        export PKG_MANAGER="apt-get"
    else
        printf "\n\e[1;31m[ERROR]\e[m Do not take into account this system.\n";
    fi

# Install ansible collection defined in ./meta/ee-requirements.yml
collections:
    @printf "\e[1;34m[INFO]\e[m ## Install Ansible Collections dependencies ##\n"
    @ansible-galaxy install -r ../../meta/ee-requirements.yml
    @printf "\e[1;32m[OK]\e[m Ansible Collections installed.\n"

# Install pythons packages defined in ./meta/ee-requirements.txt
pythons:
    @printf "\e[1;34m[INFO]\e[m ## Install Pip ##\n"
    @python3 -m venv ../../venv
    @curl --silent --show-error --retry 5 https://bootstrap.pypa.io/get-pip.py | ../../venv/bin/python3
    @printf "\e[1;32m[OK]\e[m Pip installed.\n"
    @printf "\e[1;34m[INFO]\e[m ## Install Pip packages ##\n"
    @../../venv/bin/python3 -m pip install -r ../../meta/ee-requirements.txt
    @printf "\e[1;32m[OK]\e[m Pip packages installed.\n"

# Install packages rpm/dep defined in ./meta/ee-bindeps.txt
bindeps: (_get_pkg_manager)
    @printf "\e[1;34m[INFO]\e[m ## Install Bindeps package ##\n"
    @../../venv/bin/python3 -m pip install bindep
    @printf "\e[1;34m[INFO]\e[m ## Install Bindeps dependencies ##\n"
    @for i in $( ../../venv/bin/python3 -m bindep -bf ../../meta/ee-bindeps.txt ); do echo "### $$i ###"; sudo ${PKG_MANAGER} install -y $$i; done
    @printf "\e[1;32m[OK]\e[m All packages installed.\n"

# Install admin kubernetes commands
arkade:
    ./arkade.sh