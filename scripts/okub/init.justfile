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
# IF `just init pxe`
PXE_SERVER         :=  env_var_or_default('PXE_SERVER', `hostname -i`)
# IF MASTERS greater or equal to 3
RENDEZVOUS_IP      :=  env_var_or_default('RENDEZVOUS_IP', `hostname -i`)
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', "00:ef:44:21:e6:m1 00:ef:44:21:e6:m2 00:ef:44:21:e6:m3")
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', "00:ef:44:21:e6:w1 00:ef:44:21:e6:w2")
IP_MASTERS         :=  env_var_or_default('IP_MASTERS', "xx.xx.xx.x1 xx.xx.xx.x2 xx.xx.xx.x3")
IP_WORKERS         :=  env_var_or_default('IP_WORKERS', "xx.xx.xx.x4 xx.xx.xx.x5")
DNS_SERVER         :=  env_var_or_default('DNS_SERVER', "8.8.8.8")

# Download Openshift-install in {{OKUB_INSTALL_PATH}}/bin
binaries:
    #!/usr/bin/env bash
    set -e
    # OKD or OCP
    if [[ "{{RELEASE_VERSION}}" == "latest"  && "{{PRODUCT}}" == "okd" ]]; then
        export GET_VERSION=$(curl -sS https://api.github.com/repos/okd-project/okd/releases/latest | jq -r .tag_name)
        printf "\n\e[1;34m[INFO]\e[m Get OKD latest installer: ${GET_VERSION}\n";
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [[ "{{RELEASE_VERSION}}" == "latest"  && "{{PRODUCT}}" == "ocp" ]]; then
        export GET_VERSION=$(curl -sS https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/ | grep -oP 'openshift-install-linux-\K[0-9\.]+' | sort -V | tail -n 1 | sed 's/\.$//')
        printf "\n\e[1;34m[INFO]\e[m Get OCP stable installer: ${GET_VERSION}\n";
        export URL_OC_CLIENT="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-client-linux.tar.gz"
        export URL_OCP_INSTALL="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-install-linux.tar.gz"
    elif [[ "{{PRODUCT}}" == "okd" && $(echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$"; echo $?) -eq 0 ]]; then
        printf "\n\e[1;34m[INFO]\e[m Get OKD {{RELEASE_VERSION}} installer\n";
        export GET_VERSION=$(curl -sS https://api.github.com/repos/okd-project/okd/tags | jq -r --arg version "{{RELEASE_VERSION}}" '.[].name | select(startswith($version))' | sort -V | tail -n 1) 
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [[ "{{PRODUCT}}" == "ocp" && $(echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$"; echo $?) -eq 0 ]]; then
        printf "\n\e[1;34m[INFO]\e[m Get OCP stable-{{RELEASE_VERSION}} installer\n";
        export GET_VERSION="stable-{{RELEASE_VERSION}}"
        export URL_OC_CLIENT="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-client-linux.tar.gz"
        export URL_OCP_INSTALL="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-install-linux.tar.gz"
    else
        printf "\n\e[1;31m[ERROR]\e[m The choosen version does not match follwing regex ^4\.[0-9]+$.\n";
        exit 1
    fi
    # Butane
    BU_VERSION=$(curl -sS https://api.github.com/repos/coreos/butane/releases/latest | jq -r .tag_name)
    URL_BUTANE="https://github.com/coreos/butane/releases/download/${BU_VERSION}/butane-x86_64-unknown-linux-gnu"
    # Download bin
    mkdir -p {{OKUB_INSTALL_PATH}}/bin
    if [ ! -f {{OKUB_INSTALL_PATH}}/bin/oc ]; then
        printf "\e[1;34m[INFO]\e[m Download OC\n";
        curl -L ${URL_OC_CLIENT} -o {{OKUB_INSTALL_PATH}}/bin/oc.tar.gz
        cd {{OKUB_INSTALL_PATH}}/bin; tar -xzf oc.tar.gz; cd -
    fi
    if [ ! -f {{OKUB_INSTALL_PATH}}/bin/openshift-install ]; then
        printf "\e[1;34m[INFO]\e[m Download Openshift-install\n";
        curl -L ${URL_OCP_INSTALL} -o {{OKUB_INSTALL_PATH}}/bin/openshift-install.tar.gz
        cd {{OKUB_INSTALL_PATH}}/bin; tar -xzf openshift-install.tar.gz && rm -f openshift-install.tar.gz; cd -
    fi
    if [ ! -f {{OKUB_INSTALL_PATH}}/bin/butane ]; then
        printf "\e[1;34m[INFO]\e[m Download Butane\n";
        curl -L ${URL_BUTANE} -o {{OKUB_INSTALL_PATH}}/bin/butane
        chmod 700 {{OKUB_INSTALL_PATH}}/bin/butane
    fi
    # Display results
    printf "\e[1;34m[INFO]\e[m Openshift-install version:\n";
    cd {{OKUB_INSTALL_PATH}}/bin; ./openshift-install version; cd -

# Generate SSH-keys in {{OKUB_INSTALL_PATH}}/.ssh
keys:
    #!/usr/bin/env bash
    printf "\e[1;34m[INFO]\e[m Generate SSH keys:\n";
    mkdir -p "{{OKUB_INSTALL_PATH}}/.ssh"
    ssh-keygen -q -t rsa -b 4096 -f "{{OKUB_INSTALL_PATH}}/.ssh/id_rsa" -N "" <<<'\nn\n' || printf "\n\e[1;34m[INFO]\e[m SSH already exist!\n"

# Init an install-config.yaml in {{OKUB_INSTALL_PATH}}
install-config:
    #!/usr/bin/env bash
    mkdir -p "{{OKUB_INSTALL_PATH}}"

    printf "\e[1;34m[INFO]\e[m Generate install-config:\n";

    #install-config
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

# Init an agent-config.yaml in {{OKUB_INSTALL_PATH}}
agent-config:
    #!/usr/bin/env bash
    if [[ {{ MASTERS }} -ge 3 ]]; then
    printf "\e[1;34m[INFO]\e[m Generate agent-config:\n";
    #agent-config
    cat > "{{OKUB_INSTALL_PATH}}/agent-config.yaml" <<EOF
    apiVersion: v1alpha1
    kind: AgentConfig
    metadata:
      name: sno-cluster
    rendezvousIP: 192.168.111.80
    EOF
    fi

# Update install-config.yaml
update-install-config:
    #!/usr/bin/env bash
    printf "\e[1;34m[INFO]\e[m Update install-config:\n";
    export SSH_KEY=$(cat {{OKUB_INSTALL_PATH}}/.ssh/id_rsa.pub)
    export PULLSECRET="'$(cat $HOME/.docker/config.json | jq -c .)'"
    # install-config.yaml
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

# Update agent-config.yaml
update-agent-config:
    #!/usr/bin/env bash
    if [ -f {{OKUB_INSTALL_PATH}}/agent-config.yaml ]; then
      printf "\e[1;34m[INFO]\e[m Update agent-config:\n";
      # agent-config.yaml
      yq -i '.metadata.name = "{{ CLUSTER_NAME }}"' {{OKUB_INSTALL_PATH}}/agent-config.yaml
      yq -i '.rendezvousIP = "{{ RENDEZVOUS_IP }}"' {{OKUB_INSTALL_PATH}}/agent-config.yaml

      # Static IP - https://docs.openshift.com/container-platform/4.14/installing/installing_with_agent_based_installer/preparing-to-install-with-agent-based-installer.html#static-networking
      if [[ {{ DHCP_BOOL }} == "FALSE" ]]; then

        # master config
        for ((i=0; i<MASTERS; i++)); do 
          INDEX=$((i + 1));
          MAC_ADRESS_LIST=({{ MACADRESS_MASTERS }})
          MAC_ADRESS=${MAC_ADRESS_LIST[i]}
          IP_ADDRESS_LIST=({{ IP_MASTERS }})
          IP_ADDRESS=${IP_ADDRESS_LIST[i]}

          yq -i '.hosts += [{
              "hostname": "{{CLUSTER_NAME}}-master0'${INDEX}'",
              "role": "master",
              "interfaces": [{
                  "name": "eno1",
                  "macAddress": "'${MAC_ADRESS}'"
              }],
              "dns-resolver": {
                  "config": {
                      "server": ["{{DNS_SERVER}}"]
                  }
              },
              "networkConfig": {
                  "interfaces": [{
                      "name": "eno1",
                      "type": "ethernet",
                      "state": "up",
                      "mac-address": "'${MAC_ADRESS}'",
                      "ipv4": {
                          "enabled": true,
                          "address": [{
                              "ip": "'${IP_ADDRESS}'",
                              "prefix-length": 23
                          }],
                          "dhcp": false
                      }
                  }]
              }
          }]' {{OKUB_INSTALL_PATH}}/agent-config.yaml          
        done

        # worker config
        for ((i=0; i<WORKERS; i++)); do 
          INDEX=$((i + 1));
          MAC_ADRESS_LIST=({{ MACADRESS_WORKERS }})
          MAC_ADRESS=${MAC_ADRESS_LIST[i]}
          IP_ADDRESS_LIST=({{ IP_WORKERS }})
          IP_ADDRESS=${IP_ADDRESS_LIST[i]}

          yq -i '.hosts += [{
              "hostname": "{{CLUSTER_NAME}}-worker0'${INDEX}'",
              "role": "worker",
              "interfaces": [{
                  "name": "eno1",
                  "macAddress": "'${MAC_ADRESS}'"
              }],
              "dns-resolver": {
                  "config": {
                      "server": ["{{DNS_SERVER}}"]
                  }
              },
              "networkConfig": {
                  "interfaces": [{
                      "name": "eno1",
                      "type": "ethernet",
                      "state": "up",
                      "mac-address": "'${MAC_ADRESS}'",
                      "ipv4": {
                          "enabled": true,
                          "address": [{
                              "ip": "'${IP_ADDRESS}'",
                              "prefix-length": 23
                          }],
                          "dhcp": false
                      }
                  }]
              }
          }]' {{OKUB_INSTALL_PATH}}/agent-config.yaml          
        done
      fi
    fi

# Set Platform
platform:
    #!/usr/bin/env bash
    echo "none or baremetal"

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
        {{OKUB_INSTALL_PATH}}/bin/openshift-install create single-node-ignition-config --dir {{OKUB_INSTALL_PATH}}
    else
        {{OKUB_INSTALL_PATH}}/bin/openshift-install create ignition-configs --dir {{OKUB_INSTALL_PATH}}
    fi

# Get and create CoreOS iso corresponding to current OKD/OCP version 
iso:
    #!/usr/bin/env bash
    if [ -f {{OKUB_INSTALL_PATH}}/agent-config.yaml ]; then
        # Agent Based
        {{OKUB_INSTALL_PATH}}/bin/openshift-install agent create image --dir {{OKUB_INSTALL_PATH}} 
    else        
        # UPI method
        if [ ! -f {{OKUB_INSTALL_PATH}}/rhcos-live.iso ]; then
            URL_COREOS_ISO=$({{OKUB_INSTALL_PATH}}/bin/openshift-install coreos print-stream-json | jq -r .architectures.x86_64.artifacts.metal.formats.iso.disk.location)
            printf "\e[1;34m[INFO]\e[m Download ${URL_COREOS_ISO##*/} as rhcos-live.iso\n";
            curl -L ${URL_COREOS_ISO} -o {{OKUB_INSTALL_PATH}}/rhcos-live.iso
        fi
        if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then
            WHICH_IGNITION=bootstrap-in-place-for-live-iso.ign
        else
            WHICH_IGNITION=bootstrap.ign
        fi
        export COREOS_INSTALLER="podman run --privileged --pull always --rm -v /dev:/dev -v {{OKUB_INSTALL_PATH}}:/data -w /data quay.io/coreos/coreos-installer:release"
        ${COREOS_INSTALLER} iso ignition embed -fi "${WHICH_IGNITION}" "rhcos-live.iso"
    fi
    printf "\e[1;32m[OK]\e[m RHCOS iso generated with ignition.\n"

# Get and create CoreOS pxe corresponding to current OKD/OCP version
pxe:
    #!/usr/bin/env bash
    if [ -f {{OKUB_INSTALL_PATH}}/agent-config.yaml ]; then
        # Agent Based 
        {{OKUB_INSTALL_PATH}}/bin/openshift-install agent create pxe-files --dir {{OKUB_INSTALL_PATH}}
    else
        # UPI method
        URL_COREOS_KERNEL=$({{OKUB_INSTALL_PATH}}/bin/openshift-install coreos print-stream-json | jq -r .architectures.x86_64.artifacts.metal.formats.pxe.kernel.location)
        URL_COREOS_INITRAMFS=$({{OKUB_INSTALL_PATH}}/bin/openshift-install coreos print-stream-json | jq -r .architectures.x86_64.artifacts.metal.formats.pxe.initramfs.location)
        URL_COREOS_ROOTFS=$({{OKUB_INSTALL_PATH}}/bin/openshift-install coreos print-stream-json | jq -r .architectures.x86_64.artifacts.metal.formats.pxe.rootfs.location)

        # Kernel
        if [ ! -f {{OKUB_INSTALL_PATH}}/${URL_COREOS_KERNEL##*/} ]; then
        printf "\e[1;34m[INFO]\e[m Download ${URL_COREOS_KERNEL##*/}\n";
        curl -L ${URL_COREOS_KERNEL} -o {{OKUB_INSTALL_PATH}}/${URL_COREOS_KERNEL##*/}
        fi

        # Initramfs
        if [ ! -f {{OKUB_INSTALL_PATH}}/${URL_COREOS_INITRAMFS##*/} ]; then
        printf "\e[1;34m[INFO]\e[m Download ${URL_COREOS_INITRAMFS##*/}\n";
        curl -L ${URL_COREOS_INITRAMFS} -o {{OKUB_INSTALL_PATH}}/${URL_COREOS_INITRAMFS##*/}
        fi

        # Rootfs
        if [ ! -f {{OKUB_INSTALL_PATH}}/${URL_COREOS_ROOTFS##*/} ]; then
        printf "\e[1;34m[INFO]\e[m Download ${URL_COREOS_ROOTFS##*/}\n";
        curl -L ${URL_COREOS_ROOTFS} -o {{OKUB_INSTALL_PATH}}/${URL_COREOS_ROOTFS##*/}
        fi

        # Config
        if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then
            WHICH_IGNITION=bootstrap-in-place-for-live-iso.ign
        else
            WHICH_IGNITION=bootstrap.ign
        fi

        if [[ ! {{ DHCP_BOOL }} == "FALSE" ]]; then NETWORK_CONFIG="ip=eno1:dhcp"; fi

        cat > "{{OKUB_INSTALL_PATH}}/pxeboot.conf" <<EOF
        DEFAULT pxeboot
        TIMEOUT 20
        PROMPT 0
        LABEL pxeboot
            KERNEL http://{{ PXE_SERVER }}/${URL_COREOS_KERNEL##*/} 
            APPEND initrd=http://{{ PXE_SERVER }}/${URL_COREOS_INITRAMFS##*/} coreos.live.rootfs_url=http://{{ PXE_SERVER }}/${URL_COREOS_ROOTFS##*/} coreos.inst.install_dev=/dev/sda coreos.inst.ignition_url=http://{{ PXE_SERVER }}/${WHICH_IGNITION} ${NETWORK_CONFIG}
        EOF
    fi
    printf "\e[1;32m[OK]\e[m PXE config generated and iso downloaded.\n"