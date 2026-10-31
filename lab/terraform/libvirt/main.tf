locals {
  owner        = replace(var.run_id, "-", "")
  resource_tag = "okub-lab-${substr(local.owner, 0, 12)}"
}

resource "libvirt_network" "lab" {
  name      = "${local.resource_tag}-net"
  mode      = "nat"
  addresses = [var.bridge_cidr]
  autostart = false

  dhcp { enabled = true }
  dns {
    enabled    = true
    local_only = false
    hosts {
      hostname = "api.${var.cluster_domain}"
      ip       = var.api_vip
    }
    hosts {
      hostname = "api-int.${var.cluster_domain}"
      ip       = var.api_vip
    }
  }

  dnsmasq_options {
    options {
      option_name  = "address"
      option_value = "/apps.${var.cluster_domain}/${var.ingress_vip}"
    }
  }
}

resource "libvirt_volume" "root" {
  name   = "${local.resource_tag}-root.qcow2"
  pool   = var.storage_pool
  size   = var.disk_gib * 1024 * 1024 * 1024
  format = "qcow2"
}

resource "libvirt_domain" "sno" {
  name        = "${local.resource_tag}-sno"
  description = "Okub validation lab owner=${var.run_id}"
  memory      = var.memory_mib
  vcpu        = var.vcpu
  running     = true
  autostart   = false
  firmware    = var.firmware

  disk { volume_id = libvirt_volume.root.id }

  dynamic "disk" {
    for_each = var.boot_method == "iso" ? [var.iso_path] : []
    content { file = disk.value }
  }

  network_interface {
    network_id     = libvirt_network.lab.id
    mac            = upper(var.host_mac)
    wait_for_lease = false
  }

  boot_device { dev = var.boot_method == "pxe" ? ["network", "hd"] : ["cdrom", "hd"] }
  cpu { mode = "host-passthrough" }
  graphics {
    type           = "vnc"
    autoport       = true
    listen_type    = "address"
    listen_address = "127.0.0.1"
  }
  console {
    type        = "pty"
    target_type = "serial"
    target_port = "0"
  }
}
