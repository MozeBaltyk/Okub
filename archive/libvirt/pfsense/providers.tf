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
  uri = "qemu:///system"
}