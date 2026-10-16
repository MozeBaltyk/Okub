output "pfsense_ips" {
  value = libvirt_domain.pfsense.network_interface[*].addresses
}
