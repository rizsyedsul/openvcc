variable "name" {
  description = "Resource-name prefix"
  type        = string
  default     = "openvcc-stateless"
}

variable "ssh_public_key" {
  description = "SSH public key for the VMs"
  type        = string
}

variable "ingress_cidrs" {
  description = "CIDRs allowed to reach SSH and the app port"
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

variable "aws_instance_size" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.small"
}

variable "azure_instance_size" {
  description = "Azure VM size"
  type        = string
  default     = "Standard_B2s"
}

variable "openvcc_admin_token" {
  description = "Bearer token for the engine's admin API"
  type        = string
  sensitive   = true
}

variable "echo_image" {
  description = "Image reference for the echo example"
  type        = string
  default     = "ghcr.io/syedsumx/openvcc-echo:latest"
}
