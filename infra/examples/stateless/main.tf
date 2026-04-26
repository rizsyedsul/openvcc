locals {
  cloud_init = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    image     = var.echo_image
    app_port  = 8000
  })
}

module "aws_vm" {
  source         = "../../modules/aws"
  name           = "${var.name}-aws"
  region         = var.aws_region
  instance_size  = var.aws_instance_size
  ssh_public_key = var.ssh_public_key
  ingress_cidrs  = var.ingress_cidrs
  cloud_init     = local.cloud_init
  tags = {
    Stack = var.name
  }
}

module "azure_vm" {
  source         = "../../modules/azure"
  name           = "${var.name}-azure"
  region         = var.azure_region
  instance_size  = var.azure_instance_size
  ssh_public_key = var.ssh_public_key
  ingress_cidrs  = var.ingress_cidrs
  cloud_init     = local.cloud_init
  tags = {
    Stack = var.name
  }
}

resource "local_file" "openvcc_yaml" {
  filename = "${path.module}/openvcc.yaml"
  content = templatefile("${path.module}/openvcc.yaml.tftpl", {
    aws_url      = "http://${module.aws_vm.vm_public_ip}:8000"
    aws_region   = module.aws_vm.region
    azure_url    = "http://${module.azure_vm.vm_public_ip}:8000"
    azure_region = module.azure_vm.region
  })
  file_permission = "0644"
}
