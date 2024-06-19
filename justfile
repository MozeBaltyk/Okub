set shell := ["bash", "-uc"]

RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest") 
OKD_INSTALL_PATH   :=  env_var_or_default('OKD_INSTALL_PATH', "${HOME}/OKD-" + RELEASE_VERSION)
DOMAIN             :=  env_var_or_default('DOMAIN', 'okub.com')
NAME               :=  env_var_or_default('NAME', 'okd4')
MASTERS            :=  env_var_or_default('MASTERS', "1")
WORKERS            :=  env_var_or_default('WORKERS', "0") 

# Lists all available commands in the justfile.
_help:
    @printf "\nNAME\n"
    @printf "     Okub - kickstart OKD\n"
    @printf "\n"
    @printf "SYNOPSIS\n"
    @printf "     just [vars=value] recipe [arguments]... \n"
    @printf "\n"
    @just --list --unsorted
    @printf "\n"
    @printf "DEFAULT VARIABLES\n"
    @printf "     RELEASE_VERSION = {{RELEASE_VERSION}}\n"
    @printf "     OKD_INSTALL_PATH = ${HOME}/OKD-{{RELEASE_VERSION}}\n"
    @printf "\n"

# Download Openshift-install in {{OKD_INSTALL_PATH}}/bin
binaries:
    #!/usr/bin/env bash
    # Get versions
    if [[ "{{RELEASE_VERSION}}" == "latest" ]]; then 
        export GET_VERSION=$(curl -s https://api.github.com/repos/okd-project/okd/releases/latest | jq -r .tag_name)
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [ echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$" ]; then
        export GET_VERSION=$(curl -s https://api.github.com/repos/okd-project/okd/tags | jq -r --arg version "{{RELEASE_VERSION}}" '.[].name | select(startswith($version))' | sort -V | tail -n 1) 
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    else
        printf "\e[1;31m[ERROR]\e[m The choosen version ^[a-z][a-z0-9_]+$.\n";
        exit 1
    fi
    # Download bin
    mkdir -p {{OKD_INSTALL_PATH}}/bin
    curl -L ${URL_OC_CLIENT} -o {{OKD_INSTALL_PATH}}/bin/oc.tar.gz
    curl -L ${URL_OCP_INSTALL} -o {{OKD_INSTALL_PATH}}/bin/openshift-install.tar.gz
    # Untar
    cd {{OKD_INSTALL_PATH}}/bin
    tar -xzf oc.tar.gz && rm -f oc.tar.gz
    tar -xzf openshift-install.tar.gz && rm -f openshift-install.tar.gz
    # Display results
    printf "\e[1;34m[INFO]\e[m Openshift-install version:\n";
    ./openshift-install version
    printf "\e[1;34m[INFO]\e[m OC client version:\n";
    ./oc version --kubeconfig=""
    cd -

# Generate SSH-keys in {{OKD_INSTALL_PATH}}/.ssh
keys:
    #!/usr/bin/env bash
    set -e
    mkdir -p "{{OKD_INSTALL_PATH}}/.ssh"
    ssh-keygen -t rsa -b 4096 -f "{{OKD_INSTALL_PATH}}/.ssh/id_rsa" -N ""
    echo "SSH keys generated in {{OKD_INSTALL_PATH}}/.ssh"

# Init an install-config.yaml in {{OKD_INSTALL_PATH}}
install-config:
    #!/usr/bin/env bash
    mkdir -p "{{OKD_INSTALL_PATH}}"
    cat > "{{OKD_INSTALL_PATH}}/install-config.yaml" <<EOF
    apiVersion: v1
    baseDomain: example.com 
    compute: 
    - hyperthreading: Enabled 
      name: worker
      replicas: 0 
    controlPlane: 
      hyperthreading: Enabled 
      name: master
      replicas: 3 
    metadata:
      name: test 
    networking:
      clusterNetwork:
      - cidr: 10.128.0.0/14
        hostPrefix: 23 
      networkType: OVNKubernetes 
      serviceNetwork: 
      - 172.30.0.0/16
    platform:
      none: {}
    pullSecret: '{"auths": ...}' 
    sshKey: 'ssh-ed25519 AAAA...' 
    EOF

# Update install-config.yaml
update:
    #!/usr/bin/env bash
    export SSH_KEY=$(cat {{OKD_INSTALL_PATH}}/.ssh/id_rsa.pub)
    export PULLSECRET="'$(cat $HOME/.docker/config.json | jq -c .)'"
    yq -i '.baseDomain = "{{ DOMAIN }}"' {{OKD_INSTALL_PATH}}/install-config.yaml
    yq -i '.metadata.name = "{{ NAME }}"' {{OKD_INSTALL_PATH}}/install-config.yaml
    yq -i '.controlPlane.replicas = {{ MASTERS }}' {{OKD_INSTALL_PATH}}/install-config.yaml
    yq -i '.compute.[0].replicas = {{ WORKERS }}' {{OKD_INSTALL_PATH}}/install-config.yaml
    yq -i '.sshKey = env(SSH_KEY)' {{OKD_INSTALL_PATH}}/install-config.yaml
    yq -i '.pullSecret = env(PULLSECRET)' {{OKD_INSTALL_PATH}}/install-config.yaml
    # SNO - Single Node install
    if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then 
      SNO=true;
      yq -i '.bootstrapInPlace.installationDisk = "/dev/sda"' {{OKD_INSTALL_PATH}}/install-config.yaml;
    fi

# Generate manifest
manifest:
    #!/usr/bin/env bash
    cp {{OKD_INSTALL_PATH}}/install-config.yaml {{OKD_INSTALL_PATH}}/install-config-saved.yaml
    {{OKD_INSTALL_PATH}}/bin/openshift-install create manifests --dir {{OKD_INSTALL_PATH}}
    {{OKD_INSTALL_PATH}}/bin/openshift-install create ignition-configs --dir {{OKD_INSTALL_PATH}}

# Get CoreOS corresponding to current OKD version 
fcos:
    #!/usr/bin/env bash
    URL_COREOS_ISO=$({{OKD_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.iso.disk.location)
    URL_COREOS_KERNEL=$({{OKD_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.pxe.kernel.location)
    URL_COREOS_INITRAMFS=$({{OKD_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.pxe.initramfs.location)
    URL_COREOS_ROOTFS=$({{OKD_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.pxe.rootfs.location)

# Initiate an OKD install directory for the targeted version.
init:
    @just binaries
    @just keys
    @just install-config
    @just update
    @just manifest
    

