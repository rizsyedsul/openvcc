output "aws_vm_public_ip" {
  value = module.aws_vm.vm_public_ip
}

output "azure_vm_public_ip" {
  value = module.azure_vm.vm_public_ip
}

output "openvcc_yaml_path" {
  value = local_file.openvcc_yaml.filename
}

output "next_steps" {
  description = "How to bring traffic up against this stack"
  value       = <<-EOT
    1. ssh into the AWS VM and run: docker exec -it crdb cockroach init --insecure --host=localhost --port=26257
       (only needed if you used 'cockroach start' instead of 'start-single-node'; the
       provided cloud-init starts the AWS node as a single-node cluster, then Azure joins.)
    2. Run the engine locally:
         OPENVCC_ADMIN_TOKEN=devtoken openvcc engine serve --config=${local_file.openvcc_yaml.filename}
    3. Drive traffic at http://localhost:8080/notes
  EOT
}
