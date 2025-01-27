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

# Set locally
locals {
  subdomain = "${var.clusterid}.${var.domain}"
}