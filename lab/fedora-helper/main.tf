resource "libvirt_network" "wan" {
  name      = var.wan_network_name
  mode      = "nat"
  bridge    = var.wan_bridge_name
  addresses = [var.wan_cidr]
  autostart = false

  dhcp {
    enabled = true
  }

  dns {
    enabled    = true
    local_only = false
  }

  lifecycle {
    precondition {
      condition = !anytrue([
        for cidr in concat([var.lan_cidr, var.cluster_cidr, var.service_cidr], var.wan_excluded_cidrs) :
        cidrcontains(var.wan_cidr, cidrhost(cidr, 0)) || cidrcontains(cidr, cidrhost(var.wan_cidr, 0))
      ])
      error_message = "wan_cidr must not overlap LAN, cluster, service, or excluded CIDRs."
    }
  }
}

resource "libvirt_network" "lan" {
  name      = var.lan_network_name
  mode      = "none"
  bridge    = var.lan_bridge_name
  addresses = [var.lan_cidr]
  autostart = false

  dhcp {
    enabled = false
  }

  dns {
    enabled = false
  }

  # The helper owns .1, so keep libvirt's host bridge on the final usable address.
  xml {
    xslt = templatefile("${path.module}/lan-host-address.xsl.tftpl", {
      address = cidrhost(var.lan_cidr, -2)
    })
  }

  lifecycle {
    precondition {
      condition     = var.lan_network_name != var.wan_network_name && var.lan_bridge_name != var.wan_bridge_name
      error_message = "WAN and LAN network and bridge names must be different."
    }
  }
}

resource "libvirt_volume" "base_image" {
  name   = var.base_image_volume_name
  pool   = var.storage_pool
  source = var.base_image_path
  format = "qcow2"

  lifecycle {
    precondition {
      condition     = filesha256(var.base_image_path) == lower(var.base_image_sha256)
      error_message = "base_image_path content does not match base_image_sha256."
    }
  }
}

resource "libvirt_volume" "root" {
  name           = var.root_volume_name
  pool           = var.storage_pool
  base_volume_id = libvirt_volume.base_image.id
  size           = var.root_disk_size_gib * 1024 * 1024 * 1024
  format         = "qcow2"

  lifecycle {
    precondition {
      condition     = var.root_volume_name != var.base_image_volume_name
      error_message = "root_volume_name and base_image_volume_name must be different."
    }
  }
}

resource "libvirt_cloudinit_disk" "seed" {
  name           = var.seed_volume_name
  pool           = var.storage_pool
  user_data      = sensitive(file(var.cloud_init_user_data_path))
  meta_data      = sensitive(file(var.cloud_init_meta_data_path))
  network_config = sensitive(file(var.cloud_init_network_config_path))
}

resource "libvirt_domain" "helper" {
  name        = var.vm_name
  description = "Okub Fedora 44 network-services helper"
  memory      = var.memory_mib
  vcpu        = var.vcpu
  running     = true
  autostart   = false
  qemu_agent  = false

  disk {
    volume_id = libvirt_volume.root.id
  }

  cloudinit = libvirt_cloudinit_disk.seed.id

  network_interface {
    network_id     = libvirt_network.wan.id
    mac            = upper(var.wan_mac)
    wait_for_lease = false
  }

  network_interface {
    network_id     = libvirt_network.lan.id
    mac            = upper(var.lan_mac)
    wait_for_lease = false
  }

  graphics {
    type           = "vnc"
    autoport       = true
    listen_type    = "address"
    listen_address = "127.0.0.1"
  }

  console {
    type        = "pty"
    target_port = "0"
    target_type = "serial"
  }

  boot_device {
    dev = ["hd"]
  }

  cpu {
    mode = "host-passthrough"
  }

  lifecycle {
    precondition {
      condition     = lower(var.wan_mac) != lower(var.lan_mac)
      error_message = "wan_mac and lan_mac must be different."
    }
  }
}
