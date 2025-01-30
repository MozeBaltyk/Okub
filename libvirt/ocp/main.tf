# Define libvirt iso volumes for master nodes
resource "libvirt_volume" "openshift_master_iso" {
  name   = "${var.clusterid}-master-${var.product}-${var.release_version}.iso"
  pool   = "default"
  source = "/var/lib/libvirt/images/rhcos-master-${var.product}-${var.release_version}.iso"
}

# Define libvirt iso volumes for worker nodes
resource "libvirt_volume" "openshift_worker_iso" {
  name   = "${var.clusterid}-worker-${var.product}-${var.release_version}.iso"
  pool   = "default"
  source = "/var/lib/libvirt/images/rhcos-worker-${var.product}-${var.release_version}.iso"
}

# Define libvirt volumes for master nodes
resource "libvirt_volume" "master_disk" {
  for_each = { for idx, master in local.master_details : idx => master }

  name   = "${each.value.name}-disk-${var.product}-${var.release_version}.qcow2"
  pool   = "default"
  size   = 120 * 1024 * 1024 * 1024  # 120 GB in bytes
  format = "qcow2"
}

# Define libvirt volumes for worker nodes
resource "libvirt_volume" "worker_disk" {
  for_each = { for idx, worker in local.worker_details : idx => worker }

  name   = "${each.value.name}-disk-${var.product}-${var.release_version}.qcow2"
  pool   = "default"
  size   = 120 * 1024 * 1024 * 1024  # 120 GB in bytes
  format = "qcow2"
}

resource "libvirt_network" "okub" {
  name      = "okub"
  mode      = "nat"
  bridge    = "virbr9"
  autostart = true
  domain    = "${local.subdomain}"
  addresses = [var.network_cidr]
  dhcp { enabled = false }
  dns {
    enabled = true
    local_only = false

    # Loop to create DNS entries for each master
    dynamic "hosts" {
      for_each = { for idx, master in local.master_details : idx => master }
      content {
        hostname = "${hosts.value.name}.${local.subdomain}"
        ip       = hosts.value.ip
      }
    }

    # Loop to create DNS entries for each worker
    dynamic "hosts" {
      for_each = { for idx, worker in local.worker_details : idx => worker }
      content {
        hostname = "${hosts.value.name}.${local.subdomain}"
        ip       = hosts.value.ip
      }
    }

    # Loop to create DNS entries for each etcd
    dynamic "hosts" {
      for_each = { for idx, master in local.master_details : idx => master }
      content {
        hostname = "etcd-${hosts.key}.${local.subdomain}"
        ip       = hosts.value.ip
      }
    }
    hosts {
      hostname = "api.${local.subdomain}"
      ip       = local.lb_vip
    }
    hosts {
      hostname = "api-int.${local.subdomain}"
      ip       = local.lb_vip
    }
  }

  dnsmasq_options {
    options {
      option_name  = "domain"
      option_value = "${var.domain}"
    }
    options  {
      option_name = "no-hosts"
    }
    # Loop to create srv-host entries for each master
    dynamic "options" {
      for_each = { for idx, master in local.master_details : idx => master }
      content {
        option_name  = "srv-host"
        option_value = "_etcd-server-ssl._tcp.${local.subdomain},etcd-${options.key}.${local.subdomain},2380,0,10"
      }
    }
    options {
      option_name  = "address"
      option_value = "/apps.${local.subdomain}/${local.lb_vip}"
    }
  }
}

resource "libvirt_domain" "master" {
  for_each = { for idx, master in local.master_details : idx => master }
  name   = each.value.name
  vcpu   = 4
  memory = 16 * 1024

  disk {
    volume_id = libvirt_volume.master_disk[each.key].id
  }

  disk {
    file = libvirt_volume.openshift_master_iso.id
  }

  boot_device {
    dev = [ "hd", "cdrom"]
  }

  network_interface {
    network_id = libvirt_network.okub.id
    hostname   = each.value.name
    addresses  = [each.value.ip]
    mac        = each.value.mac
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

resource "libvirt_domain" "worker" {
  for_each = { for idx, worker in local.worker_details : idx => worker }
  name   = each.value.name
  vcpu   = 4
  memory = 16 * 1024

  disk {
    volume_id = libvirt_volume.worker_disk[each.key].id
  }

  disk {
    file = libvirt_volume.openshift_worker_iso.id
  }

  boot_device {
    dev = [ "hd", "cdrom"]
  }

  network_interface {
    network_id = libvirt_network.okub.id
    hostname   = each.value.name
    addresses  = [each.value.ip]
    mac        = each.value.mac
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