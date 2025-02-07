# Default one
variable "libvirt_uri" {
    description = "The connection URI used to connect to the libvirt host"
    default = "qemu:///system"
}

variable "type" {
  description = "Type of installation (e.g., pxe, iso)"
  type        = string
  default     = "iso"
}

variable "okub_install_path" {
  description = "OKUB install path"
  type        = string
  default = "/var/lib/libvirt/images"
}

# To be defined by the user
variable "product" {
  description = "Product name for the ISO"
  type        = string
}

variable "release_version" {
  description = "Release version for the ISO"
  type        = string
}

variable "network_cidr" {
  description = "Network CIDR"
  type        = string
}

variable "clusterid" {
  description = "Cluster ID"
  type        = string
}

variable "domain" {
  description = "Domain name"
  type        = string
}

variable "masters_number" {
  description = "Number of Masters"
  type        = number
}

variable "workers_number" {
  description = "Number of Workers"
  type        = number
}

variable "masters_mac_addresses" {
  description = "Masters MAC addresses"
  type    = list(string)
}

variable "workers_mac_addresses" {
  description = "Workers MAC addresses"
  type    = list(string)
}

variable "dhcp_bool" {
  description = "DHCP enabled or not"
  type        = bool
}

variable "lb_bool" {
  description = "Load balancer enabled or not"
  type        = bool
}

# Set locally
locals {
  okub_pool_path = "${var.okub_install_path}/pool"
  okub_cache_path = "${var.okub_install_path}/cache"
  subdomain = "${var.clusterid}.${var.domain}"
  master_details = tolist([
    for m in range(var.masters_number) : {
      name = format("master%02d", m + 1)
      ip   = cidrhost(var.network_cidr, m + 10)
      mac  = var.masters_mac_addresses[m]
    }])
  worker_details = tolist([
    for w in range(var.workers_number) : {
      name = format("worker%02d", w + 1)
      ip   = cidrhost(var.network_cidr, w + 20)
      mac  = var.workers_mac_addresses[w]
    }])
  lb_vip = var.masters_number == 1 ? local.master_details[0].ip : cidrhost(var.network_cidr, 3)
}
