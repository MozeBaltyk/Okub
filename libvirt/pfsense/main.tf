# Define the storage pool
resource "libvirt_pool" "pfsense_pool" {
  name = "pfsense_pool"
  type = "dir"
  target {
    path = "${var.pfsense_install_path}"
  }
}

# Define the disk volume for pfSense
resource "libvirt_volume" "pfsense_disk" {
  name   = "pfsense-disk0.qcow2"
  pool   = libvirt_pool.pfsense_pool.name
  size   = 12 * 1024 * 1024 * 1024  # 12 GB in bytes
  format = "qcow2"
}

# Define the ISO volume for pfSense
resource "libvirt_volume" "pfsense_iso" {
  name   = "netgate-installer-amd64.iso"
  pool   = libvirt_pool.pfsense_pool.name
  source = "${var.pfsense_iso_path}"
  format = "raw"
}

# Define the network interfaces
resource "libvirt_network" "bridge0" {
  name      = "virbr0"
  mode      = "bridge"
  bridge    = "virbr0"
  autostart = true
}

resource "libvirt_network" "bridge1" {
  name      = "virbr1"
  mode      = "bridge"
  bridge    = "virbr1"
  autostart = true
}

# Define the pfSense VM
resource "libvirt_domain" "pfsense" {
  name   = "pfsense"
  memory = 2048
  vcpu   = 2
  autostart = true

  disk {
    volume_id = libvirt_volume.pfsense_disk.id
    scsi = true
  }

  disk {
    volume_id = libvirt_volume.pfsense_iso.id
    file = var.pfsense_iso_path
    device = "cdrom"
  }

  network_interface {
    network_id = libvirt_network.bridge0.id
    mac = "52:54:00:00:00:01"
    hostname = "pfsense"
  }

  network_interface {
    network_id = libvirt_network.bridge1.id
    mac = "52:54:00:00:00:02"
  }

  graphics {
    type        = "vnc"
    listen_type = "address"
    listen_address = "0.0.0.0"
  }

  console {
    type        = "pty"
    target_port = "0"
    target_type = "serial"
  }

  boot_device {
    dev = [ "hd", "cdrom"]
  }

  cpu {
    mode = "host-passthrough"
  }

  # Add cloud-init for initial configuration
  cloudinit = libvirt_cloudinit_disk.pfsense_init.id
}

