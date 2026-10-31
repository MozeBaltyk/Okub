variable "run_id" {
  description = "Explicit UUID identifying the sole owner of this run."
  type        = string
  validation {
    condition     = can(regex("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", var.run_id))
    error_message = "run_id must be a lowercase RFC 4122 version-4 UUID."
  }
}

variable "libvirt_uri" {
  type    = string
  default = "qemu:///system"
  validation {
    condition     = var.libvirt_uri == "qemu:///system"
    error_message = "Only local qemu:///system is supported."
  }
}

variable "storage_pool" {
  type = string
  validation {
    condition     = trimspace(var.storage_pool) != ""
    error_message = "storage_pool must name an existing pool."
  }
}

variable "bridge_cidr" {
  type = string
  validation {
    condition     = can(cidrhost(var.bridge_cidr, 0)) && cidrhost(var.bridge_cidr, 0) != split("/", var.bridge_cidr)[0]
    error_message = "bridge_cidr must contain an IPv4 host address and prefix."
  }
}

variable "host_mac" {
  type = string
  validation {
    condition     = can(regex("^[0-9a-f]{2}(:[0-9a-f]{2}){5}$", var.host_mac))
    error_message = "host_mac must be a lowercase MAC address."
  }
}

variable "cluster_domain" {
  type = string
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$", var.cluster_domain))
    error_message = "cluster_domain must be a DNS name."
  }
}

variable "api_vip" {
  type = string
  validation {
    condition     = can(regex("^[0-9.]+$", var.api_vip))
    error_message = "api_vip must be an IPv4 address."
  }
}

variable "ingress_vip" {
  type = string
  validation {
    condition     = can(regex("^[0-9.]+$", var.ingress_vip))
    error_message = "ingress_vip must be an IPv4 address."
  }
}

variable "boot_method" {
  type = string
  validation {
    condition     = contains(["iso", "pxe"], var.boot_method)
    error_message = "boot_method must be iso or pxe."
  }
}

variable "iso_path" {
  type    = string
  default = ""
  validation {
    condition     = var.boot_method != "iso" || (startswith(var.iso_path, "/") && fileexists(var.iso_path))
    error_message = "ISO boot requires an absolute existing published ISO path."
  }
}

variable "firmware" {
  description = "Absolute path to host UEFI firmware."
  type        = string
  validation {
    condition     = fileexists(var.firmware)
    error_message = "firmware must be an existing file."
  }
}

variable "memory_mib" {
  type    = number
  default = 16384
  validation {
    condition     = var.memory_mib >= 16384 && floor(var.memory_mib) == var.memory_mib
    error_message = "memory_mib must be an integer of at least 16384."
  }
}

variable "vcpu" {
  type    = number
  default = 8
  validation {
    condition     = var.vcpu >= 4 && floor(var.vcpu) == var.vcpu
    error_message = "vcpu must be an integer of at least 4."
  }
}

variable "disk_gib" {
  type    = number
  default = 120
  validation {
    condition     = var.disk_gib >= 100 && floor(var.disk_gib) == var.disk_gib
    error_message = "disk_gib must be an integer of at least 100."
  }
}
