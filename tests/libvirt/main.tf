// fetch the release image from their mirrors
resource "libvirt_volume" "os_image" {
  name   = "${var.hostname}-os_image"
  pool   = "${var.pool}"
  source = "${local.qcow2_image}"
  format = "qcow2"
}

// Use CloudInit ISO to add ssh-key to the instance
resource "libvirt_cloudinit_disk" "commoninit" {
  name           = "${var.hostname}-commoninit.iso"
  pool           = "${var.pool}"
  user_data      = data.template_cloudinit_config.config.rendered
  network_config = data.template_file.network_config.rendered
}

resource "libvirt_network" "network" {
    name      = var.network_name
    mode      = "nat"
    bridge    = "virbr7"
    autostart = true
    domain    = local.subdomain
    addresses = [var.network_cidr]
    dhcp { enabled = false }
}

data "template_file" "user_data" {
  template = file("${path.module}/${local.cloud_init_version}/cloud_init.cfg")
  vars = {
    hostname   = var.hostname
    fqdn       = "${var.hostname}.${local.subdomain}"
    domain     = local.subdomain
    clusterid  = var.clusterid
    timezone   = var.timezone
    network_cidr = var.network_cidr
    master_details = yamlencode(local.master_details)
    worker_details = yamlencode(local.worker_details)
    bootstrap_details = yamlencode(local.bootstrap_details)
    public_key = tls_private_key.global_key.public_key_openssh
  }
}

data "template_cloudinit_config" "config" {
  gzip          = false
  base64_encode = false
  part {
    filename     = "init.cfg"
    content_type = "text/cloud-config"
    content      = data.template_file.user_data.rendered
  }
}

data "template_file" "network_config" {
  template = file("${path.module}/${local.cloud_init_version}/network_config_${var.ip_type}.cfg")
}

// Create the machine
resource "libvirt_domain" "helper" {
  # domain name in libvirt, not hostname
  name       = var.hostname
  memory     = var.memoryMB
  vcpu       = var.cpu
  autostart  = true
  qemu_agent = true

  disk {
    volume_id = libvirt_volume.os_image.id
  }

  network_interface {
    network_id = libvirt_network.network.id
    mac          = var.mac_address
    addresses    = [cidrhost(var.network_cidr, 3)]
    wait_for_lease = true
  }

  cloudinit = libvirt_cloudinit_disk.commoninit.id

  console {
    type        = "pty"
    target_port = "0"
    target_type = "serial"
  }

  graphics {
    type        = "vnc"
    listen_type = "address"
    autoport    = "true"
  }
}