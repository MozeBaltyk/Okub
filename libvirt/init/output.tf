
output "product" {
  value = var.product
}

output "clusterid" {
  value = var.clusterid
}

output "domain" {
  value = var.domain
}

output "openshift_version" {
  value = local.get_openshift_version
}

