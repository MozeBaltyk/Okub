output "domain_name" {
  description = "Exact libvirt domain owned by this state."
  value       = libvirt_domain.helper.name
}

output "domain_running" {
  description = "Observed domain power state."
  value       = libvirt_domain.helper.running
}

output "base_image_volume_id" {
  description = "Owned immutable Fedora base-image volume ID."
  value       = libvirt_volume.base_image.id
}

output "root_volume_id" {
  description = "Owned writable root volume ID."
  value       = libvirt_volume.root.id
}

output "seed_volume_id" {
  description = "Owned cloud-init seed volume ID."
  value       = libvirt_cloudinit_disk.seed.id
}

output "wan_network_name" {
  description = "Owned temporary NAT WAN network name."
  value       = libvirt_network.wan.name
}

output "wan_bridge_name" {
  description = "Host bridge created by the owned WAN network."
  value       = libvirt_network.wan.bridge
}

output "lan_network_name" {
  description = "Owned isolated LAN network name."
  value       = libvirt_network.lan.name
}

output "lan_bridge_name" {
  description = "Host bridge created by the owned LAN network."
  value       = libvirt_network.lan.bridge
}

output "lan_host_address" {
  description = "Host address on the isolated LAN bridge."
  value       = cidrhost(var.lan_cidr, -2)
}

output "helper_lan_address" {
  description = "Required helper gateway and DNS address on the isolated LAN."
  value       = cidrhost(var.lan_cidr, 1)
}
