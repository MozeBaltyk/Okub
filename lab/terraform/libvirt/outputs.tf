output "run_id" {
  value = var.run_id
}

output "domain_name" {
  value = libvirt_domain.sno.name
}

output "network_name" {
  value = libvirt_network.lab.name
}

output "root_volume_id" {
  value = libvirt_volume.root.id
}

output "boot_method" {
  value = var.boot_method
}
