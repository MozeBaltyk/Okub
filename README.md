<h1 style="text-align: center;"><code> Ansible Collection - mozebaltyk.Okub  </code></h1>

Collection to deploy OKD/OCP on baremetal

[![Releases](https://img.shields.io/github/release/mozebaltyk/Okub)](https://github.com/mozebaltyk/Okub/releases)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0/)

## Description and Prerequisites

This Project provides CLI tools to help OKD/OCP deployment with a special focus on baremetal.

1. Type of Architecture

| Topology                     | Number of control plane nodes | Number of compute nodes | vCPU         | Memory          | Storage |
|------------------------------|-------------------------------|-------------------------|--------------|-----------------|---------|
| Single-node cluster          | 1                             | 0                       | 8 vCPU cores | 16 GB of RAM    | 120 GB  |
| Single-node cluster extended | 1                             | 1 or above              | 8 vCPU cores | 16 GB of RAM    | 120 GB  |
| Compact cluster              | 3                             | 0 or 1                  | 8 vCPU cores | 16 GB of RAM    | 120 GB  |
| HA cluster                   | 3                             | 2 and above             | 8 vCPU cores | 16 GB of RAM    | 120 GB  |

Add to above list, an *helper node* to provide following services: DNS / DHCP / PXE boot / LoadBalancer (+ eventually registry)

2. Diverse installation method

We should normally count a bootstrap node, but with **Single-node installer** and **Agent-based Installer** bootstraping is handled by one master node.    

The **Single-node installer** will have an ignition file named `bootstrap-in-place-for-live-iso.ign`. This method does not have any reason to exist since it's included in the **Agent-based Installer** from version OCP 4.15. The only advantage of this method is you do not need a *rendezvousIP* and the install is completed as *bootstrap-in-place*.   

The **Agent-based Installer** will require an extra `agent-config.yaml` to setup the *rendezvousIP* which in case of DHCP, an IP address of one of the control plane. In an environment without a DHCP server, you can define IP addresses statically.

Take also into account in the `install-config.yaml` the platform arguments which allow 3 values: `none`, `baremetal` and `vsphere`.     

3. Diverse plateform option

none (for **single-node installer** the only possible option):

- DNS for `*.api.<domain>` and `apps.<domain>`

- DNS and reverse DNS for all masters and workers

- Loadbalancer for 6443 and 22623

- [Futher documentation](https://docs.openshift.com/container-platform/4.14/installing/installing_with_agent_based_installer/preparing-to-install-with-agent-based-installer.html#installation-requirements-platform-none_preparing-to-install-with-agent-based-installer)

baremetal (used for hardware with BMC or for dual stack):

- DNS for `*.api.<domain>` and `apps.<domain>`

- DHCP services to provide IP addresses to nodes during installation. ()

- PXE booting capability for nodes to load the initial install

4. Diverse Installer Outcome

- an bootable iso to burn on USB stick

- pxe boot to push on *helper server* or any other *pxe server*

## Getting started

1. Clone this project
```sh
git clone https://github.com/Namespace/example.git 
```

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
