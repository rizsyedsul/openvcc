variable "name" {
  description = "Resource-name prefix"
  type        = string
  default     = "openvcc-stateful"
}

variable "ssh_public_key" {
  description = "SSH public key for the VMs"
  type        = string
}

variable "ingress_cidrs" {
  description = "CIDRs allowed to reach SSH, app port, and CRDB"
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
  default     = "t3.medium"
}

variable "azure_instance_size" {
  description = "Azure VM size"
  type        = string
  default     = "Standard_B2ms"
}

variable "notes_image" {
  description = "Image reference for the notes example"
  type        = string
  default     = "ghcr.io/syedsumx/openvcc-notes:latest"
}

variable "crdb_image" {
  description = "CockroachDB image reference"
  type        = string
  default     = "cockroachdb/cockroach:v24.2.4"
}
