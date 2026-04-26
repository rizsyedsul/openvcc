output "vm_public_ip" {
  description = "Public IPv4 address of the VM"
  value       = aws_instance.this.public_ip
}

output "vm_private_ip" {
  description = "Private IPv4 address of the VM"
  value       = aws_instance.this.private_ip
}

output "ssh_user" {
  description = "Default SSH user for the AMI"
  value       = "ec2-user"
}

output "cloud" {
  description = "Literal cloud identifier"
  value       = "aws"
}

output "region" {
  description = "Echoed input"
  value       = var.region
}

output "vm_id" {
  description = "EC2 instance id"
  value       = aws_instance.this.id
}
