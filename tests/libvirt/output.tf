###
### Generate the hosts.ini file
###
resource "local_file" "ansible_inventory" {
  content = templatefile("../inventory/hosts.tpl",
    {
      helper_ips = libvirt_domain.helper.*.network_interface.0.addresses.0,
      helper_hostname = var.hostname,
      master_details = local.master_details,
      worker_details = local.worker_details,
      bootstrap_details = local.bootstrap_details
    }
  )
  filename = "../inventory/hosts.ini"

  depends_on = [libvirt_domain.helper]
}

output "ips" {
  # show IP, run 'tofu refresh && tofu output ips' if not populated
  value = libvirt_domain.helper.*.network_interface.0.addresses
}

output "master_details" {
  value = local.master_details
}

output "worker_details" {
  value = local.worker_details
}

output "bootstrap_details" {
  value = local.bootstrap_details
}