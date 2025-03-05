<h1 style="text-align: center;"><code> Ansible Collection - mozebaltyk.Okub  </code></h1>

Collection to deploy OKD/OCP on baremetal

[![Releases](https://img.shields.io/github/release/mozebaltyk/Okub)](https://github.com/mozebaltyk/Okub/releases)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0/)

## Description and Prerequisites

This Project provides CLI tools to help OKD/OCP deployment with a special focus on baremetal.

1. Type of Architecture and Requirements

| Topology                     | Nb of control planes | Nb of compute nodes | vCPU         | Memory       | Storage | install method |
|------------------------------|----------------------|---------------------|--------------|--------------|---------|----------------|
| Single-node cluster          | 1                    | 0                   | 4 vCPU cores | 16 GB of RAM | 120 GB  | UPI sno        |
| Single-node cluster extended | 1                    | 1 or above          | 8 vCPU cores | 16 GB of RAM | 120 GB  | UPI sno + add  |
| Compact cluster              | 3                    | 0 or 1              | 8 vCPU cores | 16 GB of RAM | 120 GB  | ABI            |
| HA cluster                   | 3                    | 2 and above         | 8 vCPU cores | 16 GB of RAM | 120 GB  | ABI            |

Add to above list, an *helper node* or *pfsense* to provide following services: DNS / DHCP / PXE boot / LoadBalancer (+ eventually registry). In case of deployment on KVM, the DNS, DHCP and TFTP are embeded in KVM to avoid changes on the host's network config.

## Getting started

1. Get a Pull Secret and set it in `.docker/config.json`

```json

```

2.  Clone this project and get inside
```sh
git clone https://github.com/mozebaltyk/Okub.git
```

* init a project

* create a helper if needed

* deploy OCP/OKD on KVM


## Troubleshootings

Few tips for troubleshooting:

```bash
cd ${OKUB_INSTALL_PATH}
export KUBECONFIG=./auth/kubeconfig
# Agent based install
./bin/openshift-install --dir . agent wait-for bootstrap-complete --log-level=info
./bin/openshift-install --dir . agent wait-for install-complete --log-level=info
# SNO and other type of install 
./bin/openshift-install --dir . wait-for bootstrap-complete --log-level=info
./bin/openshift-install --dir . wait-for install-complete --log-level=info

/bin/oc get co

# Connect in ssh to first master node
journalctl -u bootkube --no-pager | tail -50
journalctl -u kubelet --no-pager | tail -50

systemctl status bootkube -l
systemctl status kubelet -l

# Check if the certificate is still valid
[ $(jq -r '.. | objects | select(.Filename? == "tls/root-ca.crt") | .Data' .openshift_install_state.json  | base64 -d | openssl x509 -noout -startdate | cut -d= -f2 | xargs -I{} date -d {} +%s) -le $(date -d "24 hours" +%s) ] && echo OK || echo NOK
```

After first reboot, fs should be setup:

```bash
lsblk
```

Once install is finished

```bash
oc whoami --show-console
```

## TL;DR

1. Diverse installation methods

We should normally count one extra bootstrap node, but with **Single-node installer** and **Agent-based Installer** bootstraping is handled by one master node. Since this project focus on baremetal installation. So there is a benefice to not use one baremetal for bootstraping which then need to be erase and reuse as a worker but added manually.   

The **Single-node installer** will have an ignition file named `bootstrap-in-place-for-live-iso.ign`. This install method could seems outdated and the **Agent-based Installer** a better approach. But there are still some advantage left to use it, the install is completed as *bootstrap-in-place* and require only 4 vcpu instead of 8 vcpu for **Agent-based Installer**. In case of resources scarcity like running locally on laptop, it makes sense to keep this option available.    

The **Agent-based Installer** will require an extra `agent-config.yaml` to set up the *rendezvousIP*. In the case of DHCP, this will be the control-plane IP. In an environment without a DHCP server, you can define IP addresses statically. This method seems to work for OKD even though it is not present in the documentation.

2. plateform options

**Agent-based Installer** support only those 3 plateforms options below:

- *none*, the only possible option for **single-node installer** but works also on all raw install (like for a baremetal without BMC).

   Requirements for *plateform: none{}*:

      - `networkType: OVNKubernetes`

      - DNS for `*.api.<domain>` and `apps.<domain>` pointing to the Loadbalancer.

      - DNS and reverse DNS (PTR) for all masters and workers is required

      - DHCP services to provide IP addresses to nodes during installation.

      - Loadbalancer for 6443 and 22623 if not standalone install ( since `apiVIPs` and `ingressVIPs` are not defined in *none* block )

- *baremetal*, for hardware with BMC or for configuring dual stacks network.

   Requirements for *plateform: baremetal{}*:

      - if `apiVIPs` and `ingressVIPs` are defined in the config, no need for loadbalancing

      - if static IP defined then no DHCP

      - if Outcome iso - no PXE boot server

      - only DNS for `*.api.<domain>` and `apps.<domain>` is required

- *vsphere*, does not concern us since this project focus mainly on baremetal.

4. Diverse "Helpers" are present as ansible roles but the best would be to use pfsense vm or router:

- DNS = Bind server.

- DHCP = DHCP server.

- PXE server = TFTP server.

- Loadbalancer = HAproxy server.

5. Diverse Installer Outcome

- an bootable iso to burn on USB stick

- pxe boot to push on *helper server* or in the KVM embended TFTP server.

## References

* Baremetal

https://github.com/ryanhay/ocp4-metal-install/tree/master

* Agent-based

https://www.redhat.com/en/blog/meet-the-new-agent-based-openshift-installer-1

https://kapilrajyaguru.medium.com/agent-based-red-hat-openshift-cluster-install-ee33d3b9fe0e

https://docs.openshift.com/container-platform/4.14/installing/installing_with_agent_based_installer/preparing-to-install-with-agent-based-installer.html#static-networking

https://github.com/cgruver/kamarotos/blob/main/agent-install.md

* KVM

https://github.com/jmutai/ocp4_ansible

https://github.com/lgchiaretto/ocp4_setup_upi_kvm_ansible/tree/master

https://fajlinuxblog.medium.com/openshift-running-as-single-node-with-libvirt-kvm-cb615d2c43e6

* Vsphere

https://guifreelife.com/blog/2022/05/13/OpenShift-Virtualization-on-vSphere/


* Airgap

https://www.redhat.com/en/blog/red-hat-openshift-disconnected-installations

https://two-oes.medium.com/openshift-4-in-an-air-gap-disconnected-environment-part-2-installation-1dd8bf085fdd


## Roadmap
milestones:
- To deploy sone staffs
- To add flavors

Improvment:
- Add a option to chooce by url or by copy

## Authors
morze.baltyk@proton.me

## Project status
Still on developement
