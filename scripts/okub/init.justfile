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
LB_BOOL            :=  env_var_or_default('LB', "FALSE")
# IF LB_BOOL is FALSE
APIVIP             :=  env_var_or_default('APIVIP', "192.168.10.10")
INGRESSVIP         :=  env_var_or_default('INGRESSVIP', "192.168.10.11")
MACHINENETWORK     :=  env_var_or_default('MACHINENETWORK', "192.168.10.0/24")
# IF INTERNAL_REGISTRY defined
INTERNAL_REGISTRY  :=  env_var_or_default('INTERNAL_REGISTRY', "")
# IF `just init pxe`
PXE_SERVER         :=  env_var_or_default('PXE_SERVER', `hostname -i`)
# IF MASTERS greater or equal to 3 (give one of the master's ip)
RENDEZVOUS_IP      :=  env_var_or_default('RENDEZVOUS_IP', "192.168.111.11")
# STATIC NETWORK if DHCP_BOOL is FALSE
MACADRESS_MASTERS  :=  env_var_or_default('MACADRESS_MASTERS', "00:14:22:01:23:45 00:25:96:12:34:56 00:50:56:C0:00:08")
MACADRESS_WORKERS  :=  env_var_or_default('MACADRESS_WORKERS', "00:0C:29:4F:8E:35 00:1A:4B:16:01:59")
IP_MASTERS         :=  env_var_or_default('IP_MASTERS', "192.168.111.11 192.168.111.12 192.168.111.13")
IP_WORKERS         :=  env_var_or_default('IP_WORKERS', "192.168.111.14 192.168.111.15")
INTERFACE          :=  env_var_or_default('INTERFACE', "eno1")
GATEWAY            :=  env_var_or_default('GATEWAY', "192.168.111.253")
DNS_SERVER         :=  env_var_or_default('DNS_SERVER', "192.168.111.1")


# Download Openshift-install in {{OKUB_INSTALL_PATH}}/bin
binaries:
    #!/usr/bin/env bash
    set -e
    # OKD or OCP
    if [[ "{{RELEASE_VERSION}}" == "latest"  && "{{PRODUCT}}" == "okd" ]]; then
        export GET_VERSION=$(curl -sS https://api.github.com/repos/okd-project/okd/releases/latest | jq -r .tag_name)
        printf "\n\e[1;34m[INFO]\e[m Get version OKD latest installer: ${GET_VERSION}\n";
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [[ "{{RELEASE_VERSION}}" == "latest"  && "{{PRODUCT}}" == "ocp" ]]; then
        export GET_VERSION=$(curl -sS https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/ | grep -oP 'openshift-install-linux-\K[0-9\.]+' | sort -V | tail -n 1 | sed 's/\.$//')
        printf "\n\e[1;34m[INFO]\e[m Get version OCP stable installer: ${GET_VERSION}\n";
        export URL_OC_CLIENT="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-client-linux.tar.gz"
        export URL_OCP_INSTALL="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-install-linux.tar.gz"
    elif [[ "{{PRODUCT}}" == "okd" && $(echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$"; echo $?) -eq 0 ]]; then
        printf "\n\e[1;34m[INFO]\e[m Get version OKD {{RELEASE_VERSION}} installer\n";
        export GET_VERSION=$(curl -sS https://api.github.com/repos/okd-project/okd/tags | jq -r --arg version "{{RELEASE_VERSION}}" '.[].name | select(startswith($version))' | sort -V | tail -n 1) 
        export URL_OC_CLIENT="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-client-linux-${GET_VERSION}.tar.gz"
        export URL_OCP_INSTALL="https://github.com/okd-project/okd/releases/download/${GET_VERSION}/openshift-install-linux-${GET_VERSION}.tar.gz"
    elif [[ "{{PRODUCT}}" == "ocp" && $(echo "{{RELEASE_VERSION}}" | egrep -q "^4\.[0-9]+$"; echo $?) -eq 0 ]]; then
        printf "\n\e[1;34m[INFO]\e[m Get version OCP stable-{{RELEASE_VERSION}} installer\n";
        export GET_VERSION="stable-{{RELEASE_VERSION}}"
        export URL_OC_CLIENT="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-client-linux.tar.gz"
        export URL_OCP_INSTALL="https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${GET_VERSION}/openshift-install-linux.tar.gz"
    else
        printf "\n\e[1;31m[ERROR]\e[m The choosen version does not match following regex ^4\.[0-9]+$.\n";
        exit 1
    fi
    # Butane
    BU_VERSION=$(curl -sS https://api.github.com/repos/coreos/butane/releases/latest | jq -r .tag_name)
    URL_BUTANE="https://github.com/coreos/butane/releases/download/${BU_VERSION}/butane-x86_64-unknown-linux-gnu"
    # Download bin
    mkdir -p {{OKUB_INSTALL_PATH}}/bin
    if [ ! -f {{OKUB_INSTALL_PATH}}/bin/oc ]; then
        printf "\e[1;33m[CHANGE]\e[m Download OC\n";
        curl -L ${URL_OC_CLIENT} -o {{OKUB_INSTALL_PATH}}/bin/oc.tar.gz
        cd {{OKUB_INSTALL_PATH}}/bin; tar -xzf oc.tar.gz && rm -f oc.tar.gz; cd -
    fi
    if [ ! -f {{OKUB_INSTALL_PATH}}/bin/openshift-install ]; then
        printf "\e[1;33m[CHANGE]\e[m Download Openshift-install\n";
        curl -L ${URL_OCP_INSTALL} -o {{OKUB_INSTALL_PATH}}/bin/openshift-install.tar.gz
        cd {{OKUB_INSTALL_PATH}}/bin; tar -xzf openshift-install.tar.gz && rm -f openshift-install.tar.gz; cd -
    fi
    if [ ! -f {{OKUB_INSTALL_PATH}}/bin/butane ]; then
        printf "\e[1;33m[CHANGE]\e[m Download Butane\n";
        curl -L ${URL_BUTANE} -o {{OKUB_INSTALL_PATH}}/bin/butane
        chmod 700 {{OKUB_INSTALL_PATH}}/bin/butane
    fi

    # Install nmstatectl
    if ! command -v nmstatectl  >/dev/null 2>&1; then
        if [ -f /etc/redhat-release ] ; then
          printf "\e[1;33m[CHANGE]\e[m Installing nmstatectl package...\n"
          sudo dnf install -y /usr/bin/nmstatectl
        elif [ -f /etc/debian_version ] ; then
          printf "\e[1;33m[WARNING]\e[m no nmstatectl package for ubuntu, Cargo is needed as a prerequisite to compile...\n"
          printf "\e[1;33m[CHANGE]\e[m Installing nmstatectl package...\n"
          git clone https://github.com/nmstate/nmstate.git {{OKUB_INSTALL_PATH}}/bin/nmstate
          cd {{OKUB_INSTALL_PATH}}/bin/nmstate; sudo PREFIX=/usr make install; cd -
          sudo rm -rf {{OKUB_INSTALL_PATH}}/bin/nmstate
        fi
    fi

    # Display results
    printf "\e[1;34m[INFO]\e[m Openshift-install version:\n";
    cd {{OKUB_INSTALL_PATH}}/bin; ./openshift-install version; cd -
    printf "\e[1;34m[INFO]\e[m nmstatectl version:\n";
    nmstatectl version

# Generate SSH-keys in {{OKUB_INSTALL_PATH}}/.ssh
keys:
    #!/usr/bin/env bash
    mkdir -p "{{OKUB_INSTALL_PATH}}/.ssh"
    ssh-keygen -q -t rsa -b 4096 -f "{{OKUB_INSTALL_PATH}}/.ssh/id_rsa" -N "" <<<'\nn\n' && printf "\e[1;33m[CHANGE]\e[m SSH keys Generated\n" || printf "\n\e[1;34m[INFO]\e[m SSH keys already exist!\n"

# Init an install-config.yaml in {{OKUB_INSTALL_PATH}}
install-config:
    #!/usr/bin/env bash
    mkdir -p "{{OKUB_INSTALL_PATH}}"

    printf "\e[1;33m[CHANGE]\e[m Generate install-config:\n";

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
    printf "\e[1;33m[CHANGE]\e[m Generate agent-config:\n";
    #agent-config
    cat > "{{OKUB_INSTALL_PATH}}/agent-config.yaml" <<EOF
    apiVersion: v1alpha1
    kind: AgentConfig
    metadata:
      name: ocp-cluster
    rendezvousIP: 192.168.111.80
    EOF
    fi

# Update install-config.yaml
update-install-config:
    #!/usr/bin/env bash
    printf "\e[1;33m[CHANGE]\e[m Update install-config:\n";
    export SSH_KEY=$(cat {{OKUB_INSTALL_PATH}}/.ssh/id_rsa.pub)
    if [ ! -z {{ INTERNAL_REGISTRY }} ]; then
      export PULLSECRET="'$(jq '{ "auths": { "{{ INTERNAL_REGISTRY }}": .auths["{{ INTERNAL_REGISTRY }}"], "quay.io": .auths["quay.io"], "cloud.openshift.com": .auths["cloud.openshift.com"], "registry.redhat.io": .auths["registry.redhat.io"], "registry.connect.redhat.com": .auths["registry.connect.redhat.com"] }}' $HOME/.docker/config.json | jq -c .)'"      
    else
      export PULLSECRET="'$(jq '{ "auths": { "quay.io": .auths["quay.io"], "cloud.openshift.com": .auths["cloud.openshift.com"], "registry.redhat.io": .auths["registry.redhat.io"], "registry.connect.redhat.com": .auths["registry.connect.redhat.com"] }}' $HOME/.docker/config.json | jq -c .)'"
    fi
    # install-config.yaml
    yq -i '.baseDomain = "{{ DOMAIN }}"' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.metadata.name = "{{ CLUSTER_NAME }}"' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.controlPlane.replicas = {{ MASTERS }}' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.compute.[0].replicas = {{ WORKERS }}' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.sshKey = env(SSH_KEY)' {{OKUB_INSTALL_PATH}}/install-config.yaml
    yq -i '.pullSecret = env(PULLSECRET)' {{OKUB_INSTALL_PATH}}/install-config.yaml

    # SNO - Single Node install
    if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then 
      SNO=TRUE;
      yq -i '.bootstrapInPlace.installationDisk = "/dev/sda"' {{OKUB_INSTALL_PATH}}/install-config.yaml;
    fi

    # Internal Registry
    if [ ! -z {{ INTERNAL_REGISTRY }} ]; then
      printf "\e[1;34m[INFO]\e[m Get CA from internal registry:\n";
      export REGISTRY_CA=$(openssl s_client -showcerts -connect {{ INTERNAL_REGISTRY }} </dev/null 2>/dev/null|openssl x509 -outform PEM)
      printf "\e[1;33m[CHANGE]\e[m Update install-config with internal registry:\n";
    cat >> "{{OKUB_INSTALL_PATH}}/install-config.yaml" <<EOF
    imageContentSources:
    - mirrors:
      - {{ INTERNAL_REGISTRY }}/ocp4/openshift4
      source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
    - mirrors:
      - {{ INTERNAL_REGISTRY }}/ocp4/openshift4
      source: quay.io/openshift-release-dev/ocp-release
    EOF
    yq -i '.additionalTrustBundle = strenv(REGISTRY_CA)' {{OKUB_INSTALL_PATH}}/install-config.yaml;
    fi

# Update agent-config.yaml
update-agent-config:
    #!/usr/bin/env bash
    if [ -f {{OKUB_INSTALL_PATH}}/agent-config.yaml ]; then
      printf "\e[1;33m[CHANGE]\e[m Update agent-config:\n";
      # agent-config.yaml
      yq -i '.metadata.name = "{{ CLUSTER_NAME }}"' {{OKUB_INSTALL_PATH}}/agent-config.yaml
      yq -i '.rendezvousIP = "{{ RENDEZVOUS_IP }}"' {{OKUB_INSTALL_PATH}}/agent-config.yaml

      # Static IP
      if [[ {{ DHCP_BOOL }} == "FALSE" ]]; then

        # NMstatectl require for static ips config
        if ! command -v nmstatectl  >/dev/null 2>&1; then
          printf "\n\e[1;31m[ERROR]\e[m NMstatectl bin required on localhost!\n";
        fi

        printf "\e[1;33m[CHANGE]\e[m Update Static IPs inside agent-config:\n";
        # master config
        for ((i=0; i<MASTERS; i++)); do 
          INDEX=$(printf "%02d" $((i + 1)))
          MAC_ADRESS_LIST=({{ MACADRESS_MASTERS }})
          MAC_ADRESS=${MAC_ADRESS_LIST[i]}
          IP_ADDRESS_LIST=({{ IP_MASTERS }})
          IP_ADDRESS=${IP_ADDRESS_LIST[i]}

          yq -i '.hosts += [{
              "hostname": "{{CLUSTER_NAME}}-master'${INDEX}'",
              "role": "master",
              "interfaces": [{
                  "name": "{{INTERFACE}}",
                  "macAddress": "'${MAC_ADRESS}'"
              }],
              "networkConfig": {
                  "interfaces": [{
                      "name": "{{INTERFACE}}",
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
                  }],
                  "dns-resolver": {
                      "config": {
                          "server": ["{{DNS_SERVER}}"]
                      }
                  },
                  "routes": {
                    "config": [{
                        "destination": "0.0.0.0/0",
                        "next-hop-address": "{{GATEWAY}}",
                        "next-hop-interface": "{{INTERFACE}}",
                        "table-id": "254"
                    }]
                  }
              }
          }]' {{OKUB_INSTALL_PATH}}/agent-config.yaml          
        done

        # worker config
        for ((i=0; i<WORKERS; i++)); do 
          INDEX=$(printf "%02d" $((i + 1)))
          MAC_ADRESS_LIST=({{ MACADRESS_WORKERS }})
          MAC_ADRESS=${MAC_ADRESS_LIST[i]}
          IP_ADDRESS_LIST=({{ IP_WORKERS }})
          IP_ADDRESS=${IP_ADDRESS_LIST[i]}

          yq -i '.hosts += [{
              "hostname": "{{CLUSTER_NAME}}-worker'${INDEX}'",
              "role": "worker",
              "interfaces": [{
                  "name": "{{INTERFACE}}",
                  "macAddress": "'${MAC_ADRESS}'"
              }],
              "networkConfig": {
                  "interfaces": [{
                      "name": "{{INTERFACE}}",
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
                  }],
                  "dns-resolver": {
                      "config": {
                          "server": ["{{DNS_SERVER}}"]
                      }
                  },
                  "routes": {
                    "config": [{
                        "destination": "0.0.0.0/0",
                        "next-hop-address": "{{GATEWAY}}",
                        "next-hop-interface": "{{INTERFACE}}",
                        "table-id": "254"
                    }]
                  }
              }
          }]' {{OKUB_INSTALL_PATH}}/agent-config.yaml          
        done
      fi
    fi

# Set Platform
platform:
    #!/usr/bin/env bash
    # SNO - Single Node install
    if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then SNO="TRUE"; else SNO="FALSE"; fi

    if [[ {{ LB_BOOL }} == "FALSE" && ${SNO} != "TRUE" ]]; then
      printf "\e[1;33m[CHANGE]\e[m Update install-config with baremetal plateform config\n";
      yq -i 'del(.platform.none) | .platform.baremetal = {}' {{OKUB_INSTALL_PATH}}/install-config.yaml
      yq -i '.platform.baremetal.apiVIPs = [ "{{ APIVIP }}" ]' {{OKUB_INSTALL_PATH}}/install-config.yaml
      yq -i '.platform.baremetal.ingressVIPs = [ "{{ INGRESSVIP }}" ]' {{OKUB_INSTALL_PATH}}/install-config.yaml
      yq -i '.networking.machineNetwork = [{"cidr": "{{ MACHINENETWORK }}"}]' {{OKUB_INSTALL_PATH}}/install-config.yaml

      if [[ {{ DHCP_BOOL }} == "FALSE" ]]; then
        # master config
        for ((i=0; i<MASTERS; i++)); do 
          INDEX=$(printf "%02d" $((i + 1)))
          MAC_ADRESS_LIST=({{ MACADRESS_MASTERS }})
          MAC_ADRESS=${MAC_ADRESS_LIST[i]}
          IP_ADDRESS_LIST=({{ IP_MASTERS }})
          IP_ADDRESS=${IP_ADDRESS_LIST[i]}
          yq -i '.platform.baremetal.hosts += [{ 
                      "name": "{{CLUSTER_NAME}}-master'${INDEX}'",
                      "role": "master",
                      "bootMACAddress": "'${MAC_ADRESS}'"
                  }]' {{OKUB_INSTALL_PATH}}/install-config.yaml
        done

        # worker config
        for ((i=0; i<WORKERS; i++)); do 
          INDEX=$(printf "%02d" $((i + 1)))
          MAC_ADRESS_LIST=({{ MACADRESS_WORKERS }})
          MAC_ADRESS=${MAC_ADRESS_LIST[i]}
          IP_ADDRESS_LIST=({{ IP_WORKERS }})
          IP_ADDRESS=${IP_ADDRESS_LIST[i]}
          yq -i '.platform.baremetal.hosts += [{ 
                      "name": "{{CLUSTER_NAME}}-worker'${INDEX}'",
                      "role": "worker",
                      "bootMACAddress": "'${MAC_ADRESS}'"
                  }]' {{OKUB_INSTALL_PATH}}/install-config.yaml
        done
      fi
    fi

# Save install-config and agent-config
saved:
    #!/usr/bin/env bash
    mkdir -p {{OKUB_INSTALL_PATH}}/saved
    [ -f {{OKUB_INSTALL_PATH}}/install-config.yaml ] && cp {{OKUB_INSTALL_PATH}}/install-config.yaml {{OKUB_INSTALL_PATH}}/saved/install-config.yaml || echo "no install-config.yaml to be saved"
    [ -f {{OKUB_INSTALL_PATH}}/agent-config.yaml ] && cp {{OKUB_INSTALL_PATH}}/agent-config.yaml {{OKUB_INSTALL_PATH}}/saved/agent-config.yaml || echo "no agent-config.yaml to be saved"

# Generate manifest
manifest:
    #!/usr/bin/env bash
    set -e
    if [ ! -f {{OKUB_INSTALL_PATH}}/agent-config.yaml ]; then    
      # Manifest
      {{OKUB_INSTALL_PATH}}/bin/openshift-install create manifests --dir {{OKUB_INSTALL_PATH}}

      # Ignition
      if [[ {{ MASTERS }} -eq 1 && {{ WORKERS }} -eq 0 ]]; then
        SNO=TRUE;
        {{OKUB_INSTALL_PATH}}/bin/openshift-install create single-node-ignition-config --dir {{OKUB_INSTALL_PATH}}
      else
        {{OKUB_INSTALL_PATH}}/bin/openshift-install create ignition-configs --dir {{OKUB_INSTALL_PATH}}
      fi
    fi

# Get and create CoreOS iso corresponding to current OKD/OCP version 
iso:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Generate iso.\n";

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
    set -e
    printf "\e[1;34m[INFO]\e[m Generate PXE boot files\n";

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

        if [[ ! {{ DHCP_BOOL }} == "FALSE" ]]; then NETWORK_CONFIG="ip={{INTERFACE}}:dhcp"; fi

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

# Watch and Wait for OKD/OCP install to complete
wait level:
    #!/usr/bin/env bash
    set -e
    if [ -f {{OKUB_INSTALL_PATH}}/agent.x86_64.iso ]; then
       # Agent based install
      {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} agent wait-for bootstrap-complete --log-level={{level}}
      {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} agent wait-for install-complete --log-level={{level}}
    else
       # SNO and other type of install 
       {{OKUB_INSTALL_PATH}}/bin/openshift-install --dir {{OKUB_INSTALL_PATH}} wait-for install-complete --log-level={{level}}
    fi
