output "aws_vm_public_ip" {
  value = module.aws_vm.vm_public_ip
}

output "azure_vm_public_ip" {
  value = module.azure_vm.vm_public_ip
}

output "openvcc_yaml_path" {
  description = "Local path to a ready-to-use openvcc.yaml that points at both VMs"
  value       = local_file.openvcc_yaml.filename
}
