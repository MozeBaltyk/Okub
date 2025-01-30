provider "local" {}

resource "local_file" "install-config" {
  content = templatefile("./template/install-config.yaml.tftpl",
    {
        clusterid = var.clusterid,
        domain = var.domain,
        network_cidr = var.network_cidr,
        masters_number = var.masters_number,
        workers_number = var.workers_number,
        lb_vip = local.lb_vip,
        master_details = local.master_details,
        worker_details = local.worker_details,
        public_key = tls_private_key.global_key.public_key_openssh
    }
  )
  filename = "${var.okub_install_path}/install-config.yaml"
}

resource "local_file" "agent-config" {
  content = templatefile("./template/agent-config.yaml.tftpl",
    {
        clusterid = var.clusterid,
        domain = var.domain,
        network_cidr = var.network_cidr,
        masters_number = var.masters_number,
        workers_number = var.workers_number,
        lb_vip = local.lb_vip,
        master_details = local.master_details,
        worker_details = local.worker_details
    }
  )
  filename = "${var.okub_install_path}/agent-config.yaml"
}