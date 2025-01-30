<h1 style="text-align: center;"><code> Ansible Collection - mozebaltyk.Okub  </code></h1>

Collection to deploy OKD/OCP on baremetal

[![Releases](https://img.shields.io/github/release/mozebaltyk/Okub)](https://github.com/mozebaltyk/Okub/releases)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0/)

## Description and Prerequisites

This Project provides CLI tools to help OKD/OCP deployment with a special focus on baremetal.

1. Type of Architecture and Requirements

| Topology                     | Number of control plane nodes | Number of compute nodes | vCPU         | Memory          | Storage |
|------------------------------|-------------------------------|-------------------------|--------------|-----------------|---------|
| Single-node cluster          | 1                             | 0                       | 8 vCPU cores | 16 GB of RAM    | 120 GB  |
| Single-node cluster extended | 1                             | 1 or above              | 8 vCPU cores | 16 GB of RAM    | 120 GB  |
| Compact cluster              | 3                             | 0 or 1                  | 8 vCPU cores | 16 GB of RAM    | 120 GB  |
| HA cluster                   | 3                             | 2 and above             | 8 vCPU cores | 16 GB of RAM    | 120 GB  |

Add to above list, an *helper node* to provide following services: DNS / DHCP / PXE boot / LoadBalancer (+ eventually registry)

2. Diverse installation method

We should normally count a bootstrap node, but with **Single-node installer** and **Agent-based Installer** bootstraping is handled by one master node.    

The **Single-node installer** will have an ignition file named `bootstrap-in-place-for-live-iso.ign`. This method does not have any reason to exist anymore since it's included in the **Agent-based Installer** but the only advantage is you do not need a *rendezvousIP* and the install is completed as *bootstrap-in-place*.   

The **Agent-based Installer** will require an extra `agent-config.yaml` to setup the *rendezvousIP* which in case of DHCP will be the one of the control-plane IP. In an environment without a DHCP server, you can define IP addresses statically. This method seems to work for OKD even though is not present in documentation.

Take also into account in the `install-config.yaml` the platform arguments which allow 3 values: `none`, `baremetal` and `vsphere`.     

3. plateform options

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

4. Diverse Helper are present in script to meet requirement above:

- DNS = Bind server.

- DHCP = DHCP server.

- PXE server = HTTPD server.

- Loadbalancer = HAproxy server.

5. Diverse Installer Outcome

- an bootable iso to burn on USB stick

- pxe boot to push on *helper server* or any other *pxe server*

## Getting started

1. Clone this project
```sh
git clone https://github.com/mozebaltyk/Okub.git
```

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
