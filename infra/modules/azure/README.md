# `infra/modules/azure`

Provisions a single Ubuntu 24.04 LTS Azure VM behind a Standard public IP, with an NSG exposing only `app_port` and `22` to `ingress_cidrs`. Implements the [Open VCC IaC contract](../_contract.md).

## Usage

```hcl
module "azure_vm" {
  source         = "../../modules/azure"
  name           = "openvcc-demo-azure"
  region         = "eastus"
  instance_size  = "Standard_B2s"
  ssh_public_key = file("~/.ssh/id_ed25519.pub")
  ingress_cidrs  = ["1.2.3.4/32"]
  cloud_init     = file("./cloud-init.yaml")
  tags = {
    env = "demo"
  }
}
```

## What it creates

- Resource group, VNet `10.30.0.0/16`, subnet, public IP (Standard SKU).
- NSG allowing SSH and `app_port` from `ingress_cidrs`.
- Linux VM (admin user `azureuser`, key-based SSH only) with a 30 GB OS disk.

## What it does **not** do

- No HTTPS termination — pair with a CDN/WAF in front.
- No VNet peering / private cross-cloud networking.
- No VMSS; one VM per module call.
