variable "libvirt_uri" {
  description = "Explicit libvirt connection URI."
  type        = string
  default     = "qemu:///system"

  validation {
    condition     = var.libvirt_uri == "qemu:///system"
    error_message = "libvirt_uri must be qemu:///system for local managed networking."
  }
}

variable "vm_name" {
  description = "Unique libvirt domain name owned by this state."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$", var.vm_name))
    error_message = "vm_name must be 1-63 characters using letters, digits, dot, underscore, or hyphen."
  }
}

variable "wan_network_name" {
  description = "Explicit name for the managed NAT WAN network."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$", var.wan_network_name))
    error_message = "wan_network_name must be 1-63 characters using letters, digits, dot, underscore, or hyphen."
  }
}

variable "lan_network_name" {
  description = "Explicit name for the managed isolated LAN network."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$", var.lan_network_name))
    error_message = "lan_network_name must be 1-63 characters using letters, digits, dot, underscore, or hyphen."
  }
}

variable "storage_pool" {
  description = "Name of an existing libvirt storage pool. The module never creates a pool."
  type        = string

  validation {
    condition     = trimspace(var.storage_pool) != ""
    error_message = "storage_pool must name an existing libvirt storage pool."
  }
}

variable "base_image_path" {
  description = "Absolute path to the Go-downloaded and verified Fedora Cloud 44 Generic x86_64 qcow2 image."
  type        = string

  validation {
    condition = (
      startswith(var.base_image_path, "/") &&
      endswith(lower(var.base_image_path), ".qcow2") &&
      fileexists(var.base_image_path)
    )
    error_message = "base_image_path must be an absolute path to an existing .qcow2 file."
  }
}

variable "base_image_sha256" {
  description = "Expected SHA-256 for the Fedora image, previously verified against Fedora's signed metadata by Go."
  type        = string

  validation {
    condition     = can(regex("^[0-9A-Fa-f]{64}$", var.base_image_sha256))
    error_message = "base_image_sha256 must contain exactly 64 hexadecimal characters."
  }
}

variable "base_image_volume_name" {
  description = "Explicit name for the immutable managed Fedora base-image volume."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,126}\\.qcow2$", var.base_image_volume_name))
    error_message = "base_image_volume_name must be a safe .qcow2 volume name."
  }
}

variable "root_volume_name" {
  description = "Explicit name for the writable helper root volume."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,126}\\.qcow2$", var.root_volume_name))
    error_message = "root_volume_name must be a safe .qcow2 volume name."
  }
}

variable "seed_volume_name" {
  description = "Explicit name for the cloud-init NoCloud seed volume."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,128}\\.iso$", var.seed_volume_name))
    error_message = "seed_volume_name must be a safe .iso volume name."
  }
}

variable "cloud_init_user_data_path" {
  description = "Absolute path to Go-rendered, secret-free cloud-config user-data."
  type        = string

  validation {
    condition     = startswith(var.cloud_init_user_data_path, "/") && fileexists(var.cloud_init_user_data_path)
    error_message = "cloud_init_user_data_path must be an absolute path to an existing file."
  }
}

variable "cloud_init_meta_data_path" {
  description = "Absolute path to Go-rendered NoCloud meta-data."
  type        = string

  validation {
    condition     = startswith(var.cloud_init_meta_data_path, "/") && fileexists(var.cloud_init_meta_data_path)
    error_message = "cloud_init_meta_data_path must be an absolute path to an existing file."
  }
}

variable "cloud_init_network_config_path" {
  description = "Absolute path to Go-rendered cloud-init network-config v2."
  type        = string

  validation {
    condition     = startswith(var.cloud_init_network_config_path, "/") && fileexists(var.cloud_init_network_config_path)
    error_message = "cloud_init_network_config_path must be an absolute path to an existing file."
  }
}

variable "wan_cidr" {
  description = "Dedicated RFC1918 IPv4 CIDR for the owned NAT WAN."
  type        = string

  validation {
    condition = can(cidrnetmask(var.wan_cidr)) && can(regex("/2[4-9]$", var.wan_cidr)) && anytrue([
      for private_cidr in ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"] :
      cidrcontains(private_cidr, cidrhost(var.wan_cidr, 0)) && cidrcontains(private_cidr, cidrhost(var.wan_cidr, -1))
    ])
    error_message = "wan_cidr must be a valid RFC1918 IPv4 CIDR from /24 through /29."
  }
}

variable "lan_cidr" {
  description = "Cluster machine IPv4 CIDR for the owned isolated LAN."
  type        = string

  validation {
    condition     = can(cidrnetmask(var.lan_cidr)) && can(regex("/2[4-9]$", var.lan_cidr))
    error_message = "lan_cidr must be a valid IPv4 CIDR from /24 through /29."
  }
}

variable "wan_bridge_name" {
  description = "Deterministic Linux bridge name for the owned WAN network."
  type        = string
  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$", var.wan_bridge_name))
    error_message = "wan_bridge_name must be a valid Linux interface name of at most 15 characters."
  }
}

variable "lan_bridge_name" {
  description = "Deterministic Linux bridge name for the owned LAN network."
  type        = string
  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$", var.lan_bridge_name))
    error_message = "lan_bridge_name must be a valid Linux interface name of at most 15 characters."
  }
}

variable "cluster_cidr" {
  description = "Cluster pod IPv4 CIDR, excluded from the temporary WAN."
  type        = string

  validation {
    condition     = can(cidrnetmask(var.cluster_cidr))
    error_message = "cluster_cidr must be a valid IPv4 CIDR."
  }
}

variable "service_cidr" {
  description = "Cluster service IPv4 CIDR, excluded from the temporary WAN."
  type        = string

  validation {
    condition     = can(cidrnetmask(var.service_cidr))
    error_message = "service_cidr must be a valid IPv4 CIDR."
  }
}

variable "wan_excluded_cidrs" {
  description = "Additional IPv4 CIDRs the WAN must not overlap, including existing host and libvirt networks."
  type        = list(string)

  validation {
    condition     = alltrue([for cidr in var.wan_excluded_cidrs : can(cidrnetmask(cidr))])
    error_message = "wan_excluded_cidrs entries must be valid IPv4 CIDRs."
  }
}

variable "wan_mac" {
  description = "Locally administered unicast MAC address for the WAN interface."
  type        = string

  validation {
    condition     = can(regex("^[0-9A-Fa-f][26AaEe]:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}$", var.wan_mac))
    error_message = "wan_mac must be a locally administered unicast MAC address."
  }
}

variable "lan_mac" {
  description = "Locally administered unicast MAC address for the LAN interface."
  type        = string

  validation {
    condition     = can(regex("^[0-9A-Fa-f][26AaEe]:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}$", var.lan_mac))
    error_message = "lan_mac must be a locally administered unicast MAC address."
  }
}

variable "vcpu" {
  description = "Number of virtual CPUs."
  type        = number
  default     = 2

  validation {
    condition     = var.vcpu >= 1 && var.vcpu <= 64 && floor(var.vcpu) == var.vcpu
    error_message = "vcpu must be an integer from 1 through 64."
  }
}

variable "memory_mib" {
  description = "VM memory in MiB."
  type        = number
  default     = 2048

  validation {
    condition     = var.memory_mib >= 1024 && floor(var.memory_mib) == var.memory_mib
    error_message = "memory_mib must be an integer of at least 1024."
  }
}

variable "root_disk_size_gib" {
  description = "Writable helper root volume size in GiB."
  type        = number
  default     = 20

  validation {
    condition     = var.root_disk_size_gib >= 8 && floor(var.root_disk_size_gib) == var.root_disk_size_gib
    error_message = "root_disk_size_gib must be an integer of at least 8."
  }
}
