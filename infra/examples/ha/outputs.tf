output "app_aws_public_ip" {
  value = module.app_aws.vm_public_ip
}

output "app_azure_public_ip" {
  value = module.app_azure.vm_public_ip
}

output "engine_aws_public_ip" {
  description = "AWS-side engine. Point one DNS A record at this."
  value       = module.engine_aws.vm_public_ip
}

output "engine_azure_public_ip" {
  description = "Azure-side engine. Point a second DNS A record at this."
  value       = module.engine_azure.vm_public_ip
}

output "next_steps" {
  description = "How to drive traffic and how to wire DNS-level HA in front"
  value       = <<-EOT
    The two engine VMs are running openvcc in front of the two app VMs.

    Smoke-test each engine independently:
      curl -i http://${module.engine_aws.vm_public_ip}/
      curl -i http://${module.engine_azure.vm_public_ip}/

    Production-grade HA: create two A records at the same hostname
    (e.g. api.example.com), one per engine IP, and configure your DNS
    provider to health-check each (Route 53 health checks, Cloudflare
    DNS, NS1) so a dead engine drops out of rotation.

    Run a chaos verification (from your laptop, against either engine):
      OPENVCC_ADMIN_TOKEN=<your-token> \\
      openvcc chaos run \\
        --admin-url=http://${module.engine_aws.vm_public_ip}:8081 \\
        --proxy-url=http://${module.engine_aws.vm_public_ip}/ \\
        --fail=aws --duration=30s -o failover-proof.json
  EOT
}
