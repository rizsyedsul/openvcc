variable "name" {
  description = "Resource-name prefix and tag value"
  type        = string
}

variable "region" {
  description = "AWS region (e.g. us-east-1)"
  type        = string
}

variable "instance_size" {
  description = "EC2 instance type (e.g. t3.small)"
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key inserted into the VM via cloud-init"
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
  description = "cloud-init / user_data; empty disables"
  type        = string
  default     = ""
}

variable "tags" {
  description = "Tags applied to every resource that supports them"
  type        = map(string)
  default     = {}
}
