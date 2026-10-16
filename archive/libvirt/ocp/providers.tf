terraform {
  required_providers {
    libvirt = {
      source = "dmacvicar/libvirt"
      version = "0.8.1"
    }
  }
}

terraform {
  required_version = ">= 0.12"
}

provider "libvirt" {
  # Configuration du fournisseur libvirt
  uri = var.libvirt_uri
  # uri = "qemu:///system"
  # uri = "qemu:///session"
  # uri = "qemu:///session?socket=/run/user/1000/libvirt/virtqemud-sock"
}