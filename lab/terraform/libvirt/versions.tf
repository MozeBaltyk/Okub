terraform {
  required_version = ">= 1.8.0, < 2.0.0"

  required_providers {
    libvirt = {
      source  = "dmacvicar/libvirt"
      version = "0.8.3"
    }
  }

  backend "local" {}
}

provider "libvirt" {
  uri = var.libvirt_uri
}
