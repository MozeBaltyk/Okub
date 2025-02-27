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

variable "tftpboot_path" {
  description = "tftpboot path"
  type        = string
  default     = "/var/lib/tftpboot/"
}

# To be defined by the user
variable "okub_install_path" {
  description = "OKUB install path"
  type        = string
}

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
  okub_pool_path      = "/srv/${var.clusterid}/pool"
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

  # DNS config depending on lb_bool
  gateway_ip = cidrhost(var.network_cidr, 1)
  lb_vip = local.gateway_ip

  # dnsmasq base
  dnsmasq_options_base = concat(
  [{
    option_name  = "domain"
    option_value = local.subdomain
  },
  { 
    option_name = "no-hosts"
    option_value = ""
  }],
  [for master in local.master_details : {
    option_name  = "address"
    option_value = "/${master.name}.${local.subdomain}/${master.ip}"
  }],
  [for worker in local.worker_details : {
    option_name  = "address"
    option_value = "/${worker.name}}.${local.subdomain}/${worker.ip}"
  }]
  )

  # When lb_bool is false
  dns_hosts = var.lb_bool ? [] : concat(
    [
      for master in local.master_details : {
        hostname = "api.${local.subdomain}"
        ip       = master.ip
      }
    ],
    [
      for master in local.master_details : {
        hostname = "api-int.${local.subdomain}"
        ip = master.ip
      }
    ],
  )

  # When lb_bool is true
  dnsmasq_options_lb = var.lb_bool ? [
    {
      option_name  = "address"
      option_value = "/api.${var.domain}/${local.lb_vip}"
    },
    {
      option_name  = "address"
      option_value = "/api-int.${var.domain}/${local.lb_vip}"
    },
    {
      option_name  = "address"
      option_value = "/apps.${var.domain}/${local.lb_vip}"
    },
  ] : []

  # Loop to create srv-host entries for _etcd server (not needed in newer versions of OCP)
  dnsmasq_options_etcd = concat(
    [ for idx, master in local.master_details : {
      option_name  = "srv-host"
      option_value = "_etcd-server-ssl._tcp.${local.subdomain},etcd-${idx}.${local.subdomain},2380,0,10"
    }],
    [for idx, master in local.master_details : {
      option_name  = "address"
      option_value = "/etcd-${idx}.${local.subdomain}/${master.ip}/"
    }]
  )

  # When type is pxe
  dnsmasq_options_pxe = var.type == "pxe" ? [
    {
      option_name  = "dhcp-boot"
      option_value = "boot.ipxe"
    },
    {
      option_name  = "enable-tftp"
    },
    {
      option_name  = "tftp-root"
      option_value = "${var.tftpboot_path}"
    },
    {
      option_name  = "dhcp-option"
      option_value = "66,0.0.0.0"
    }
  ] : []

  # Concatenate dnsmasq_options_base and dnsmasq_options_pxe
  dnsmasq_options = concat(local.dnsmasq_options_base, local.dnsmasq_options_lb, local.dnsmasq_options_etcd, local.dnsmasq_options_pxe)

}
