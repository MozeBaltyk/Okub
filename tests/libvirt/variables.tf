# Selecting version
variable "selected_version" {
  default = "fedora40"  # You can change this as needed to "fedora41"
}

# Mapping
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
  }
}

# Set locally
locals {
  qcow2_image = lookup(var.Versionning[var.selected_version], "os_URL", "")
}

locals {
  cloud_init_version = lookup(var.Versionning[var.selected_version], "cloud-init_version", 0)
}

# To be set
variable "hostname" { default = "test" }
variable "pool" { default = "default" }
variable "domain" { default = "ocp4.local" }
variable "ip_type" { default = "dhcp" } # dhcp is other valid type
variable "network_name" { default = "openshift4" }
variable "mac_address" { default = "52:54:00:36:14:e5" }
variable "memoryMB" { default = 1024 * 2 }
variable "cpu" { default = 1 }
variable "timezone" { default = "Europe/Paris" }