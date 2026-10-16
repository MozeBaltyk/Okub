variable "pfsense_install_path" {
  description = "Path to install pfSense"
  type        = string
  default     = "${path.module}/pfsense"
}

variable "pfsense_iso_path" {
  description = "Path to the pfSense ISO"
  type        = string
  default     = "${path.module}/netgate-installer-amd64.iso"
}