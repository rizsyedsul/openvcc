output "vm_public_ip" {
  description = "Public IPv4 address of the VM"
  value       = azurerm_public_ip.this.ip_address
}

output "vm_private_ip" {
  description = "Private IPv4 address of the VM"
  value       = azurerm_network_interface.this.private_ip_address
}

output "ssh_user" {
  description = "Default SSH user"
  value       = "azureuser"
}

output "cloud" {
  description = "Literal cloud identifier"
  value       = "azure"
}

output "region" {
  description = "Echoed input"
  value       = var.region
}

output "vm_id" {
  description = "Azure VM resource id"
  value       = azurerm_linux_virtual_machine.this.id
}
