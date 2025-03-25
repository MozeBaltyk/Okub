provider "local" {}

# Install oc
resource "null_resource" "download_and_extract_oc" {
  provisioner "local-exec" {
    quiet = true
    command = <<EOT
      mkdir -p ${var.okub_install_path}/bin

      # Install oc
      if [ ! -f ${var.okub_install_path}/bin/oc ]; then
        printf "\e[1;33m[CHANGE]\e[m Download OC\n";
        curl -Ls ${local.url_oc_client} -o ${var.okub_install_path}/bin/oc.tar.gz
        cd ${var.okub_install_path}/bin; tar -xzf oc.tar.gz && rm -f oc.tar.gz; cd -
      fi
    EOT
  }
}

# Install openshift-install
resource "null_resource" "download_and_extract_openshift_install" {
  provisioner "local-exec" {
    quiet = true
    command = <<EOT
      if [ ! -f ${var.okub_install_path}/bin/openshift-install ]; then
        printf "\e[1;33m[CHANGE]\e[m Download Openshift-install\n";
        curl -Ls ${local.url_ocp_install} -o ${var.okub_install_path}/bin/openshift-install.tar.gz
        cd ${var.okub_install_path}/bin; tar -xzf openshift-install.tar.gz && rm -f openshift-install.tar.gz; cd -
      fi
    EOT
  }
}

# Install butane
resource "null_resource" "download_and_extract_butane" {
  provisioner "local-exec" {
    quiet = true
    command = <<EOT
      if [ ! -f ${var.okub_install_path}/bin/butane ]; then
        printf "\e[1;33m[CHANGE]\e[m Download Butane\n";
        curl -Ls ${local.url_butane} -o ${var.okub_install_path}/bin/butane
        chmod 700 ${var.okub_install_path}/bin/butane
      fi  
    EOT
  }
}

# Install nmstatectl
resource "null_resource" "download_and_extract_nmstatectl" {
  provisioner "local-exec" {
    quiet = true
    command = <<EOT
      if [ -f /etc/redhat-release ] ; then
        printf "\e[1;33m[CHANGE]\e[m Installing nmstatectl package...\n"
        sudo dnf install -y /usr/bin/nmstatectl
      elif [ -f /etc/debian_version ] ; then
        printf "\e[1;33m[WARNING]\e[m no nmstatectl package for ubuntu, Cargo is needed as a prerequisite to compile...\n"
        printf "\e[1;33m[CHANGE]\e[m Installing nmstatectl package...\n"
        git clone https://github.com/nmstate/nmstate.git ${var.okub_install_path}/bin/nmstate
        cd ${var.okub_install_path}/bin/nmstate; sudo PREFIX=/usr make install; cd -
        sudo rm -rf ${var.okub_install_path}/bin/nmstate
      fi
    EOT
  }
}

# Generate template install-config.yaml
resource "local_file" "install-config" {
  depends_on = [null_resource.download_and_extract_openshift_install]
  content = templatefile("./template/install-config.yaml.tftpl",
    {
        clusterid = var.clusterid,
        domain = var.domain,
        network_cidr = var.network_cidr,
        masters_number = var.masters_number,
        workers_number = var.workers_number,
        master_details = local.master_details,
        worker_details = local.worker_details,
        public_key = tls_private_key.global_key.public_key_openssh,
        pull_secret = local.pull_secret,
        sno_install = local.sno_install,
        internal_registry_url = var.internal_registry,
        registry_ca_certificate = local.registry_ca_certificate,
        lb_bool = var.lb_bool, 
        dhcp_bool = var.dhcp_bool,
        api_vip = local.api_vip,
        ingress_vip = local.ingress_vip,
        install_disk = var.install_disk
    }
  )
  filename = "${var.okub_install_path}/install-config.yaml"

  provisioner "local-exec" {
    command = "mkdir -p ${var.okub_install_path}/saved && cp ${var.okub_install_path}/install-config.yaml ${var.okub_install_path}/saved/."
  }
}

# Generate template agent-config.yaml (if SNO false)
resource "local_file" "agent-config" {
  depends_on = [null_resource.download_and_extract_openshift_install]
  count    = local.rendezvous_ip != null ? 1 : 0
  content = templatefile("./template/agent-config.yaml.tftpl",
    {
        clusterid = var.clusterid,
        rendezvous_ip = local.rendezvous_ip,
        domain = var.domain,
        network_cidr = var.network_cidr,
        masters_number = var.masters_number,
        workers_number = var.workers_number,
        master_details = local.master_details,
        worker_details = local.worker_details,
        gateway_ip = local.gateway_ip,
        pxe_server_ip = local.pxe_server_ip,
        dns_server_ip = local.dns_server_ip,
        network_interface = var.network_interface,
        lb_bool = var.lb_bool,
        dhcp_bool = var.dhcp_bool,
        option = var.option,
    }
  )
  filename = "${var.okub_install_path}/agent-config.yaml"

  provisioner "local-exec" {
    command = "mkdir -p ${var.okub_install_path}/saved && cp ${var.okub_install_path}/agent-config.yaml ${var.okub_install_path}/saved/."
  }
}

resource "local_file" "partition_var" {
  count = var.size_partition_var != 0 ? 1 : 0

  content  = templatefile("${path.module}/template/create-a-partition-for-var.bu.tftpl", {
    ocp_version = var.release_version,
    install_disk = var.install_disk,
    size_partition = var.size_partition_var,
    role = "worker", #In compact setup or SNO, the masters are also workers so keep worker role here!
  })
  filename = "${var.okub_install_path}/bu/create-a-partition-var.bu"
}

resource "local_file" "iso_script" {
  content  = templatefile("${path.module}/template/iso.sh.tftpl", {
    okub_install_path = var.okub_install_path,
    masters_number = var.masters_number,
    workers_number = var.workers_number,
  })
  filename = "${var.okub_install_path}/bin/iso.sh"
  file_permission = "0750"
}

resource "local_file" "pxe_script" {
  content  = templatefile("${path.module}/template/pxe.sh.tftpl", {
    okub_install_path = var.okub_install_path,
    masters_number = var.masters_number,
    workers_number = var.workers_number,
    dhcp_bool = var.dhcp_bool,
    network_interface = var.network_interface,
    pxe_server_ip = local.pxe_server_ip,
    ocp_version = var.release_version,
    product = var.product,
  })
  filename = "${var.okub_install_path}/bin/pxe.sh"
  file_permission = "0750"
}

resource "local_file" "dns_script" {
  content  = templatefile("${path.module}/template/local-dns-config.sh.tftpl", {
    dns_server_ip = local.dns_server_ip,
    subdomain = local.subdomain,
  })
  filename = "${var.okub_install_path}/bin/local-dns-config.sh"
  file_permission = "0750"
}
