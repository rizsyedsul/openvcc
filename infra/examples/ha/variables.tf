variable "name" {
  description = "Resource-name prefix"
  type        = string
  default     = "openvcc-ha"
}

variable "ssh_public_key" {
  description = "SSH public key for every VM"
  type        = string
}

variable "ingress_cidrs" {
  description = "CIDRs allowed to reach SSH and the proxy/admin/metrics ports"
  type        = list(string)
}

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "azure_region" {
  description = "Azure region"
  type        = string
  default     = "eastus"
}

variable "app_instance_size" {
  description = "Instance size for the app VMs (small)"
  type        = string
  default     = "t3.small"
}

variable "azure_app_instance_size" {
  description = "Azure app VM size"
  type        = string
  default     = "Standard_B2s"
}

variable "engine_instance_size" {
  description = "AWS engine VM size"
  type        = string
  default     = "t3.small"
}

variable "azure_engine_instance_size" {
  description = "Azure engine VM size"
  type        = string
  default     = "Standard_B2s"
}

variable "echo_image" {
  description = "Image reference for the echo example"
  type        = string
  default     = "ghcr.io/syedsumx/openvcc-echo:latest"
}

variable "engine_image" {
  description = "Image reference for the openvcc engine"
  type        = string
  default     = "ghcr.io/syedsumx/openvcc:latest"
}

variable "admin_token" {
  description = "Bearer token for the engine's admin API"
  type        = string
  sensitive   = true
}
