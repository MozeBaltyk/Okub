resource "libvirt_volume" "openshift_sno_iso" {
  name   = "rhcos-${var.product}-${var.release_version}.iso"
  pool   = "default"
  source = "/var/lib/libvirt/images/rhcos-live-${var.product}-${var.release_version}.iso"
  #format = "qcow2"
}

#resource "libvirt_volume" "openshift_sno_disk" {
#  name           = "rhcos-${var.product}-${var.release_version}.qcow2"
#  size           = 32212254720  # 30 GB in bytes
#  pool           = "default"
#  base_volume_id = libvirt_volume.openshift_sno_iso.id
#}

resource "libvirt_volume" "openshift_sno_disk" {
  name   = "sno-${var.product}-${var.release_version}.qcow2"
  pool   = "default"
  size   = 120 * 1024 * 1024 * 1024  # 120 GB in bytes
  format = "qcow2"
}

resource "libvirt_network" "sno" {
  name      = "sno"
  mode      = "nat"
  bridge    = "virbr9"
  autostart = true
  domain    = "${local.subdomain}"
  addresses = [var.network_cidr]
  dhcp { enabled = false }
  dns {
    enabled = true
    local_only = false
    #hosts = {
    #  hostname = "openshift-sno.${local.subdomain}"
    #  ip       = cidrhost(var.network_cidr, 3)
    #}
  }
  dnsmasq_options {
    options {
      option_name  = "address"
      option_value = "/.api.${local.subdomain}/${cidrhost(var.network_cidr, 3)}"
    }
    options {
      option_name  = "address"
      option_value = "/*.apps.${local.subdomain}/${cidrhost(var.network_cidr, 3)}"
    }
  }
}

resource "libvirt_domain" "openshift_sno" {
  name   = "openshift-sno"
  vcpu   = 2
  memory = 6144

  disk {
    volume_id = libvirt_volume.openshift_sno_disk.id
    scsi      = "true"
    }

  disk {
    file = libvirt_volume.openshift_sno_iso.id
    }

  boot_device {
    dev = [ "hd", "cdrom"]
  }

  network_interface {
    network_id = libvirt_network.sno.id
    addresses  = [ cidrhost(var.network_cidr, 3) ]
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
