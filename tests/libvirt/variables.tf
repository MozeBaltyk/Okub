# Selecting version
variable "selected_version" {
  default = "fedora40"  # You can change this as needed to "fedora41"
}

# Mapping Versions
variable "Versionning" {
  type = map(object({
    os_name = string
    os_version_short = number
    os_version_long = string
    os_URL= string
    cloud-init_version = number
  }))
  default = {
    fedora40 = {
      os_name = "fedora"
      os_version_short = 40
      os_version_long = "40.1.14"
      os_URL= "https://dl.fedoraproject.org/pub/fedora/linux/releases/40/Cloud/x86_64/images/Fedora-Cloud-Base-Generic.x86_64-40-1.14.qcow2"
      cloud-init_version = 23.4
    }
    fedora41 = {
      os_name = "fedora"
      os_version_short = 41
      os_version_long = "41.1.4"
      os_URL= "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2"
      cloud-init_version = 24.4
    }
    rhel9 ={
      os_name = "redhat"
      os_version_short = 9
      os_version_long = "9.5"
      os_URL= "/var/lib/libvirt/images/rhel95.qcow2"
      cloud-init_version = 24.4
    }
  }
}

# Set locally
locals {
  qcow2_image = lookup(var.Versionning[var.selected_version], "os_URL", "")
  cloud_init_version = lookup(var.Versionning[var.selected_version], "cloud-init_version", 0)
  subdomain = "${var.clusterid}.${var.domain}"
  master_ips = [for i in range(var.masters_number) : cidrhost(var.network_cidr, i + 10)]
  worker_ips = [for i in range(var.workers_number) : cidrhost(var.network_cidr, i + 20)]
  bootstrap_ip = cidrhost(var.network_cidr, 7)
}

variable "mac_addresses" {
  type    = list(string)
  default = ["52:54:00:36:14:e5", "52:54:00:36:14:e6", "52:54:00:36:14:e7"]
}

locals {
  master_details = [
    for i in range(var.masters_number) : {
      name = format("master%02d", i + 1)
      ip   = cidrhost(var.network_cidr, i + 10)
      mac  = var.mac_addresses[i]
    }
  ]
}

output "master_details" {
  value = local.master_details
}

# To be set
variable "hostname" { default = "helper" }
variable "pool" { default = "default" }
variable "clusterid" { default = "ocp4" }
variable "domain" { default = "local" }
variable "ip_type" { default = "dhcp" } # dhcp is other valid type
variable "network_name" { default = "openshift4" }
variable "network_cidr" { default= "192.168.100.0/24" }
variable "mac_address" { default = "52:54:00:36:14:e5" }
variable "memoryMB" { default = 1024 * 4 }
variable "cpu" { default = 2 }
variable "timezone" { default = "Europe/Paris" }
variable "masters_number" { default = 3 }
variable "workers_number" { default = 2 }
