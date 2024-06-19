set shell := ["bash", "-uc"]

PRODUCT            :=  env_var_or_default('PRODUCT', "okd")
RELEASE_VERSION    :=  env_var_or_default('RELEASE_VERSION', "latest")
OKUB_INSTALL_PATH  :=  env_var_or_default('OKUB_INSTALL_PATH', "${HOME}/" + PRODUCT + "-" + RELEASE_VERSION)
DOMAIN             :=  env_var_or_default('DOMAIN', 'okub.com')
CLUSTER_NAME       :=  env_var_or_default('CLUSTER_NAME', 'okub4')
MASTERS            :=  env_var_or_default('MASTERS', "1")
WORKERS            :=  env_var_or_default('WORKERS', "0")

# Lists all available commands in the justfile.
_help:
    @printf "\nNAME\n"
    @printf "     Okub - kickstart OKD/OCP\n"
    @printf "\n"
    @printf "SYNOPSIS\n"
    @printf "     just [vars=value] recipe [arguments]... \n"
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

# Download Openshift-install in {{OKUB_INSTALL_PATH}}/bin
binaries:
    #!/usr/bin/env bash
    # Get versions
    if [[ "{{RELEASE_VERSION}}" == "latest"  && "{{PRODUCT}}" == "okd" ]]; then
        printf "\e[1;34m[INFO]\e[m Get OKD latest installer\n";
        export GET_VERSION=$(curl -s https://api.github.com/repos/okd-project/okd/releases/latest | jq -r .tag_name)
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [[ "{{RELEASE_VERSION}}" == "latest"  && "{{PRODUCT}}" == "ocp" ]]; then
        printf "\e[1;34m[INFO]\e[m Get OCP latest installer\n";
        export GET_VERSION="stable"
        export URL_OC_CLIENT="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-client-linux.tar.gz"
        export URL_OCP_INSTALL="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-install-linux.tar.gz"
    elif [[ "{{PRODUCT}}" == "okd" && $(echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$"; echo $?) -eq 0 ]]; then
        printf "\e[1;34m[INFO]\e[m Get OKD {{RELEASE_VERSION}} installer\n";
        export GET_VERSION=$(curl -s https://api.github.com/repos/okd-project/okd/tags | jq -r --arg version "{{RELEASE_VERSION}}" '.[].name | select(startswith($version))' | sort -V | tail -n 1) 
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [[ "{{PRODUCT}}" == "ocp" && $(echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$"; echo $?) -eq 0 ]]; then
        printf "\e[1;34m[INFO]\e[m Get OKD {{RELEASE_VERSION}} installer\n";
        export GET_VERSION="stable-{{RELEASE_VERSION}}"
        export URL_OC_CLIENT="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-client-linux.tar.gz"
        export URL_OCP_INSTALL="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-install-linux.tar.gz"
    elif [ echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$" ]; then
        printf "\e[1;34m[INFO]\e[m Get OKD latest installer\n";
    else
        printf "\e[1;31m[ERROR]\e[m The choosen version does not match follwing regex ^4\.[0-9]+$.\n";
        exit 1
    fi
    # Download bin
    mkdir -p {{OKUB_INSTALL_PATH}}/bin
    curl -L ${URL_OC_CLIENT} -o {{OKUB_INSTALL_PATH}}/bin/oc.tar.gz
    curl -L ${URL_OCP_INSTALL} -o {{OKUB_INSTALL_PATH}}/bin/openshift-install.tar.gz
    # Untar
    cd {{OKUB_INSTALL_PATH}}/bin
    tar -xzf oc.tar.gz && rm -f oc.tar.gz
    tar -xzf openshift-install.tar.gz && rm -f openshift-install.tar.gz
    # Display results
    printf "\e[1;34m[INFO]\e[m Openshift-install version:\n";
    ./openshift-install version
    cd -

# Generate SSH-keys in {{OKUB_INSTALL_PATH}}/.ssh
keys:
    #!/usr/bin/env bash
    set -e
    mkdir -p "{{OKUB_INSTALL_PATH}}/.ssh"
    ssh-keygen -t rsa -b 4096 -f "{{OKUB_INSTALL_PATH}}/.ssh/id_rsa" -N ""
    echo "SSH keys generated in {{OKUB_INSTALL_PATH}}/.ssh"

# Init an install-config.yaml in {{OKUB_INSTALL_PATH}}
install-config:
    #!/usr/bin/env bash
    mkdir -p "{{OKUB_INSTALL_PATH}}"
    cat > "{{OKUB_INSTALL_PATH}}/install-config.yaml" <<EOF
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
    export SSH_KEY=$(cat {{OKUB_INSTALL_PATH}}/.ssh/id_rsa.pub)
    export PULLSECRET="'$(cat $HOME/.docker/config.json | jq -c .)'"
    yq -i '.baseDomain = "{{ DOMAIN }}"' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.metadata.name = "{{ CLUSTER_NAME }}"' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.controlPlane.replicas = {{ MASTERS }}' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.compute.[0].replicas = {{ WORKERS }}' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.sshKey = env(SSH_KEY)' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.pullSecret = env(PULLSECRET)' {{OKUB_INSTALL_PATH}}/install-config.yaml
    # SNO - Single Node install
    if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then 
      SNO=true;
      yq -i '.bootstrapInPlace.installationDisk = "/dev/sda"' {{OKUB_INSTALL_PATH}}/install-config.yaml;
    fi

# Generate manifest
manifest:
    #!/usr/bin/env bash
    # Saving
    cp {{OKUB_INSTALL_PATH}}/install-config.yaml {{OKUB_INSTALL_PATH}}/install-config-saved.yaml
    # Manifest
    {{OKUB_INSTALL_PATH}}/bin/openshift-install create manifests --dir {{OKUB_INSTALL_PATH}}
    # Ignition
    if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then
        SNO=true;
        {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir=ocp create single-node-ignition-config --dir {{OKUB_INSTALL_PATH}}
    else
        {{OKUB_INSTALL_PATH}}/bin/openshift-install create ignition-configs --dir {{OKUB_INSTALL_PATH}}
    fi

# Get CoreOS corresponding to current OKD version 
fcos:
    #!/usr/bin/env bash
    URL_COREOS_ISO=$({{OKUB_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.iso.disk.location)
    URL_COREOS_KERNEL=$({{OKUB_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.pxe.kernel.location)
    URL_COREOS_INITRAMFS=$({{OKUB_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.pxe.initramfs.location)
    URL_COREOS_ROOTFS=$({{OKUB_INSTALL_PATH}}/openshift-install coreos print-stream-json | jq .architectures.x86_64.artifacts.metal.formats.pxe.rootfs.location)

# Initiate an OKD install directory for the targeted version.
init:
    @just binaries
    @just keys
    @just install-config
    @just update
    @just manifest
