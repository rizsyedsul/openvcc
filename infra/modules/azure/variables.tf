variable "name" {
  description = "Resource-name prefix and tag value"
  type        = string
}

variable "region" {
  description = "Azure region (e.g. eastus)"
  type        = string
}

variable "instance_size" {
  description = "Azure VM size (e.g. Standard_B2s)"
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key for the admin user"
  type        = string
}

variable "ingress_cidrs" {
  description = "CIDRs allowed to reach the app port and SSH"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "app_port" {
  description = "TCP port the example app listens on"
  type        = number
  default     = 8000
}

variable "cloud_init" {
  description = "cloud-init / custom_data; empty disables"
  type        = string
  default     = ""
}

variable "tags" {
  description = "Tags applied to every resource that supports them"
  type        = map(string)
  default     = {}
}
