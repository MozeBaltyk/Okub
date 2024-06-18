###
### Provider part
###
terraform {
  required_providers {
    digitalocean = {
      source = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }
  backend "s3" {
    skip_region_validation      = true
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_requesting_account_id  = true
    use_path_style              = true
    skip_s3_checksum            = true
    endpoints = {
      s3 = "https://fra1.digitaloceanspaces.com"
    }
    region                      = "fra1"
    // bucket                   = "terraform-backend-github"
    key                         = "terraform.tfstate"
  }
}

provider "digitalocean" {
  token = var.do_token
}

data "digitalocean_ssh_key" "terraform" {
  name = "terraform"
}

###
### VPC
###
resource "digitalocean_vpc" "rkub-project-network" {
  name     = "test-${var.GITHUB_RUN_ID}-network"
  region   = var.region
}

###
### Droplet INSTANCES
###

# Droplet Instance
resource "digitalocean_droplet" "test1" {
    count = var.do_controller_count
    image = var.do_system
    name = "test1_${count.index}"
    region = var.region
    size = var.do_instance_size
    tags   = [
      "test-${var.GITHUB_RUN_ID}",
      "test1",
      "${var.do_system}",
      ]
    vpc_uuid = digitalocean_vpc.rkub-project-network.id
    ssh_keys = [
      data.digitalocean_ssh_key.terraform.id
    ]
}

output "ip_address_test1" {
  value = digitalocean_droplet.test1[*].ipv4_address
  description = "The public IP address."
}


###
### Project
###
resource "digitalocean_project" "rkub" {
  name        = "test-${var.GITHUB_RUN_ID}"
  description = "A CI project to test"
  purpose     = "TEST"
  environment = "Staging"
  resources = flatten([digitalocean_droplet.test1.*.urn, digitalocean_droplet.workers.*.urn])
}

###
### Generate the hosts.ini file
###
resource "local_file" "ansible_inventory" {
  content = templatefile("../../inventory/hosts.tpl",
    {
     test1_ips = digitalocean_droplet.test1[*].ipv4_address,
    }
  )
  filename = "../../inventory/hosts.ini"
}
