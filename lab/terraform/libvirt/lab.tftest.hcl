mock_provider "libvirt" {}

run "pxe_sno_plan" {
  command = plan

  variables {
    run_id         = "12345678-1234-4234-8234-123456789abc"
    storage_pool   = "default"
    bridge_cidr    = "192.168.122.254/24"
    host_mac       = "52:54:00:00:00:10"
    cluster_domain = "lab.example.test"
    api_vip        = "192.168.122.10"
    ingress_vip    = "192.168.122.11"
    boot_method    = "pxe"
    firmware       = "fixtures/OVMF.fd"
  }

  assert {
    condition     = libvirt_domain.sno.description == "Okub validation lab owner=12345678-1234-4234-8234-123456789abc"
    error_message = "The domain must retain the explicit owner identifier."
  }

  assert {
    condition     = libvirt_domain.sno.boot_device[0].dev[0] == "network"
    error_message = "PXE runs must boot from the production-served network path."
  }
}
