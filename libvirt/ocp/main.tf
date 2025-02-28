### Pool
resource "libvirt_pool" "okub_pool" {
  name = var.clusterid
  type = "dir"
  target {
    path = local.okub_pool_path
  }
  xml { 
    xslt = file("${path.module}/files/os_pool_permissions.xsl.tpl" ) 
  }
}

### ISO
# Define libvirt iso volumes for master nodes
resource "libvirt_volume" "openshift_master_iso" {
  count  = var.type == "iso" ? 1 : 0
  name   = "${var.clusterid}-master-${var.product}-${var.release_version}.iso"
  pool   = libvirt_pool.okub_pool.name
  source = "${local.okub_cache_path}/rhcos-master.iso"
}

# Define libvirt iso volumes for worker nodes
resource "libvirt_volume" "openshift_worker_iso" {
  count  = var.type == "iso" ? 1 : 0
  name   = "${var.clusterid}-worker-${var.product}-${var.release_version}.iso"
  pool   = libvirt_pool.okub_pool.name
  source = "${local.okub_cache_path}/rhcos-worker.iso"
}

### Disks
# Define libvirt volumes for master nodes
resource "libvirt_volume" "master_disk" {
  for_each = { for idx, master in local.master_details : idx => master }
  name   = "${each.value.name}-disk-${var.product}-${var.release_version}.qcow2"
  pool   = libvirt_pool.okub_pool.name
  size   = 120 * 1024 * 1024 * 1024  # 120 GB in bytes
  format = "qcow2"
}

# Define libvirt volumes for worker nodes
resource "libvirt_volume" "worker_disk" {
  for_each = { for idx, worker in local.worker_details : idx => worker }
  name   = "${each.value.name}-disk-${var.product}-${var.release_version}.qcow2"
  pool   = libvirt_pool.okub_pool.name
  size   = 120 * 1024 * 1024 * 1024  # 120 GB in bytes
  format = "qcow2"
}

### Networks
resource "libvirt_network" "okub" {
  name      = "okub"
  mode      = "nat"
  bridge    = "virbr-${var.clusterid}"
  autostart = true
  domain    = "${local.subdomain}"
  addresses = [var.network_cidr]
  dhcp { enabled = true }
  dns {
    local_only = true
    dynamic "hosts" {
      for_each = local.dns_hosts
      content {
        hostname = hosts.value.hostname
        ip       = hosts.value.ip
      }
    }
  }

  dnsmasq_options {
    dynamic "options" {
      for_each = local.dnsmasq_options
      content {
        option_name  = options.value["option_name"]
        option_value = options.value["option_value"]
      }
    }
  }
}

### ISO install
resource "libvirt_domain" "master_iso" {
  for_each = var.type == "iso" ? { for idx, master in local.master_details : idx => master } : {}
  name   = each.value.name
  vcpu   = each.value.cpu
  memory = each.value.memory

  disk {
    volume_id = libvirt_volume.master_disk[each.key].id
    scsi      = "true"
  }

  disk {
    file = libvirt_volume.openshift_master_iso[0].id
    scsi = "true"
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

  timeouts {
    create = "60m"
  }
  
}

resource "libvirt_domain" "worker_iso" {
  for_each = var.type == "iso" ? { for idx, worker in local.worker_details : idx => worker } : {}
  name   = each.value.name
  vcpu   = each.value.cpu
  memory = each.value.memory

  disk {
    volume_id = libvirt_volume.worker_disk[each.key].id
  }

  disk {
    file = libvirt_volume.openshift_worker_iso[0].id
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

### PXE install
resource "libvirt_domain" "master_pxe" {
  for_each = var.type == "pxe" ? { for idx, master in local.master_details : idx => master } : {}
  name   = each.value.name
  vcpu   = each.value.cpu
  memory = each.value.memory

  disk {
    volume_id = libvirt_volume.master_disk[each.key].id
    scsi     = "true"
  }

  boot_device {
    dev = [ "hd", "network"]
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

resource "libvirt_domain" "worker_pxe" {
  for_each = var.type == "pxe" ? { for idx, worker in local.worker_details : idx => worker } : {}
  name   = each.value.name
  vcpu   = each.value.cpu
  memory = each.value.memory * 1024

  disk {
    volume_id = libvirt_volume.worker_disk[each.key].id
    scsi     = "true"
  }

  boot_device {
    dev = [ "hd", "network"]
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
