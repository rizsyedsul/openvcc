# `infra/modules/aws`

Provisions a single Amazon Linux 2023 EC2 instance behind a public IP, with a hardened security group exposing only `app_port` and `22` to `ingress_cidrs`. Implements the [Open VCC IaC contract](../_contract.md).

## Usage

```hcl
module "aws_vm" {
  source         = "../../modules/aws"
  name           = "openvcc-demo-aws"
  region         = "us-east-1"
  instance_size  = "t3.small"
  ssh_public_key = file("~/.ssh/id_ed25519.pub")
  ingress_cidrs  = ["1.2.3.4/32"]
  cloud_init     = file("./cloud-init.yaml")
  tags = {
    env = "demo"
  }
}
```

## What it creates

- VPC `10.20.0.0/16` with a public subnet, IGW, and route table.
- Security group allowing SSH and `app_port` from `ingress_cidrs`.
- Key pair from `ssh_public_key`.
- One EC2 instance with IMDSv2 required and an encrypted gp3 root volume.

## What it does **not** do

- No HTTPS termination — pair with a CDN/WAF in front.
- No VPC peering / private cross-cloud networking.
- No Auto Scaling Group; one instance per module call.
