// variables that can be overriden
## variable "qcow2_image" { default = "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Server/x86_64/images/Fedora-Server-KVM-41-1.4.x86_64.qcow2" }
## variable "qcow2_image" { default = "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2" }
variable "qcow2_image" { default = "https://dl.fedoraproject.org/pub/fedora/linux/releases/40/Cloud/x86_64/images/Fedora-Cloud-Base-Generic.x86_64-40-1.14.qcow2" }
variable "hostname" { default = "test" }
variable "pool" { default = "default" }
variable "domain" { default = "ocp4.local" }
variable "ip_type" { default = "dhcp" } # dhcp is other valid type
variable "network_name" { default = "openshift4" }
variable "mac_address" { default = "52:54:00:36:14:e5" }
variable "memoryMB" { default = 1024 * 2 }
variable "cpu" { default = 1 }
variable "timezone" { default = "Europe/Paris" }