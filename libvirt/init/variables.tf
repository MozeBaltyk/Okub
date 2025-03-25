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

variable "okub_install_path" {
  description = "Path to the OKUB installation directory"
  type    = string
}

variable "dhcp_bool" {
  description = "DHCP enabled or not"
  type        = bool
}

variable "lb_bool" {
  description = "Load balancer enabled or not"
  type        = bool
}

variable "helper_bool" {
  description = "Load balancer enabled or not"
  type        = bool
}

# Defined by default
variable "internal_registry" { 
  description = "Internal registry"
  type        = string
  default     = ""
}

variable "network_interface" {
  description = "Load balancer enabled or not"
  type        = string
  default    = "ens3"
}

variable "option" {
  description = "Option for which outcome (iso/pxeboot/just manifests)"
  type        = string
  default     = ""
}

variable "install_disk" {
  description = "Target Disk to install OCP"
  type        = string
  default    = "/dev/sda"
}

variable "size_partition_var" {
  description = "Size of the partition to create for LVM storage"
  type        = number
  default     = 0 #mean all disk for coreos 
}

# Data sources
data "http" "okd_latest_release" {
  url = "https://api.github.com/repos/okd-project/okd/releases/latest"
}

data "http" "ocp_latest_release" {
  url = "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/"
}

data "http" "okd_specific_release" {
  url = "https://api.github.com/repos/okd-project/okd/tags"
}

# ocp_specific_release not defined as it's set in local 

data "http" "butane_latest_release" {
  url = "https://api.github.com/repos/coreos/butane/releases/latest"
}

# Get CA certificate from internal_registry
data "external" "fetch_ca_certificate" {
  program = ["bash", "-c", <<EOT
if [ "${var.internal_registry}" != "" ]; then
  ca_cert=$(openssl s_client -showcerts -connect ${var.internal_registry} </dev/null 2>/dev/null | openssl x509 -outform PEM)
  echo "{\"ca_certificate\": \"$ca_cert\"}"
else
  echo "{\"ca_certificate\": \"\"}"
fi
EOT
  ]
}

# Set locally
locals {
  # Extract okd latest version
  okd_latest_release_json = jsondecode(data.http.okd_latest_release.response_body)
  okd_latest_tag_name = local.okd_latest_release_json.tag_name

  # Extract ocp latest version
  ocp_latest_versions = [for v in regexall("openshift-install-linux-([0-9\\.]+)", data.http.ocp_latest_release.response_body) : replace( v[0], "/\\.$/", "" )]
  ocp_sorted_versions = sort(local.ocp_latest_versions)
  ocp_latest_tag_name = length(local.ocp_sorted_versions) > 0 ? element(local.ocp_sorted_versions, length(local.ocp_sorted_versions)-1) : null

  # Extract okd specific version
  okd_specific_release_json = jsondecode(data.http.okd_specific_release.response_body)
  okd_specific_versions = [
    for tag in local.okd_specific_release_json : tag.name
    if can(regex("^${var.release_version}", tag.name))
  ]
  okd_specific_versions_sorted = sort(local.okd_specific_versions)
  okd_specific_tag_name = length(local.okd_specific_versions_sorted) > 0 ? element(local.okd_specific_versions_sorted, length(local.okd_specific_versions_sorted)-1) : null

  # Extract ocp specific version
  ocp_specific_tag_name = "stable-${var.release_version}"

  # Extract butane latest version
  butane_latest_release_json = jsondecode(data.http.butane_latest_release.response_body)
  butane_latest_tag_name = local.butane_latest_release_json.tag_name

  # Define versions
  get_openshift_version = (
    var.release_version == "latest" && var.product == "okd" ? 
    local.okd_latest_tag_name :
    var.release_version == "latest" && var.product == "ocp" ? 
    local.ocp_latest_tag_name :
    var.product == "okd" && can(regex("^4\\.[0-9]+$", var.release_version)) ? 
    local.okd_specific_tag_name :
    var.product == "ocp" && can(regex("^4\\.[0-9]+$", var.release_version)) ? 
    local.ocp_specific_tag_name : 
    null
  )

  get_butane_version = local.butane_latest_tag_name

  # Define url from versions
  url_oc_client = (
    var.product == "okd" ? 
    "https://github.com/okd-project/okd/releases/download/${local.get_openshift_version}/openshift-client-linux-${local.get_openshift_version}.tar.gz" : 
    var.product == "ocp" ?
    "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${local.get_openshift_version}/openshift-client-linux.tar.gz" : 
    null
  )

  url_ocp_install = (
    var.product == "okd" ? 
    "https://github.com/okd-project/okd/releases/download/${local.get_openshift_version}/openshift-install-linux-${local.get_openshift_version}.tar.gz" : 
    var.product == "ocp" ? 
    "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${local.get_openshift_version}/openshift-install-linux.tar.gz" :
    null
  )

  url_butane = "https://github.com/coreos/butane/releases/download/${local.get_butane_version}/butane-x86_64-unknown-linux-gnu"

  # check SNO
  sno_install = var.masters_number == 1 && var.workers_number == 0

  # check SNO
  # sno_install = (var.masters_number == 1 && var.workers_number == 0) || var.option == "pxe"

  # Network configuration
  subdomain = "${var.clusterid}.${var.domain}"
  gateway_ip = cidrhost(var.network_cidr, 1)
  # if lb_bool is false then defined
  api_vip = var.lb_bool ? null : cidrhost(var.network_cidr, 7)
  ingress_vip = var.lb_bool ? null : cidrhost(var.network_cidr, 8)
  # if lb_bool is true then defined
  lb_vip = var.lb_bool ? local.gateway_ip : null
  # if SNO is false and iso install then defined (first master)
  rendezvous_ip = (local.sno_install && var.option == "iso") ? null : cidrhost(var.network_cidr, 10)
  # if helper is true
  helper_ip = var.helper_bool ? cidrhost(var.network_cidr, 3) : null
  dns_server_ip = var.helper_bool ? local.helper_ip : local.gateway_ip
  pxe_server_ip = var.helper_bool ? local.helper_ip : local.gateway_ip

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

  # Define pull secret
  docker_config_path    = pathexpand("~/.docker/config.json")
  docker_config_content = file(local.docker_config_path)
  docker_config_json    = jsondecode(local.docker_config_content)
  registry_ca_certificate = var.internal_registry != "" ? data.external.fetch_ca_certificate.result.ca_certificate : null
  pull_secret           = ( 
    var.internal_registry != "" ? (
      jsonencode({ auths = { (var.internal_registry) = local.docker_config_json.auths[var.internal_registry] } }) 
    ):(
      jsonencode({ auths = {
      "quay.io" = local.docker_config_json.auths["quay.io"],
      "cloud.openshift.com" = local.docker_config_json.auths["cloud.openshift.com"],
      "registry.redhat.io" = local.docker_config_json.auths["registry.redhat.io"],
      "registry.connect.redhat.com" = local.docker_config_json.auths["registry.connect.redhat.com"]
      }})
    )
  )
}