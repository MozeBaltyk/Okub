resource "libvirt_volume" "openshift_sno_iso" {
  name   = "rhcos-live.iso"
  pool   = "default"
  source = "/var/lib/libvirt/images/rhcos-live-${var.product}-${var.release_version}.iso"
  format = "raw"
}

resource "libvirt_network" "sno" {
  name      = "sno"
  mode      = "nat"
  bridge    = "virbr9"
  autostart = true
  domain    = "${local.subdomain}"
  addresses = [var.network_cidr]
  dhcp { enabled = false }
}

resource "libvirt_domain" "openshift_sno" {
  name   = "openshift-sno"
  vcpu   = 4
  memory = 8192

  disk {
    volume_id = libvirt_volume.openshift_sno_iso.id
  }

  network_interface {
    network_id = libvirt_network.sno.id
    addresses  = [cidrhost(var.network_cidr, 3)]
    wait_for_lease = true
  }

  graphics {
    type        = "vnc"
    listen_type = "address"
  }

  console {
    type        = "pty"
    target_port = "0"
    target_type = "serial"
  }

  cpu {
    mode = "host-passthrough"
  }

}
