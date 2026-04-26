locals {
  app_cloud_init = templatefile("${path.module}/cloud-init.app.yaml.tftpl", {
    image    = var.echo_image
    app_port = 8000
  })
}

module "app_aws" {
  source         = "../../modules/aws"
  name           = "${var.name}-app-aws"
  region         = var.aws_region
  instance_size  = var.app_instance_size
  ssh_public_key = var.ssh_public_key
  ingress_cidrs  = var.ingress_cidrs
  cloud_init     = local.app_cloud_init
  tags           = { Stack = var.name, Role = "app" }
}

module "app_azure" {
  source         = "../../modules/azure"
  name           = "${var.name}-app-azure"
  region         = var.azure_region
  instance_size  = var.azure_app_instance_size
  ssh_public_key = var.ssh_public_key
  ingress_cidrs  = var.ingress_cidrs
  cloud_init     = local.app_cloud_init
  tags           = { Stack = var.name, Role = "app" }
}

locals {
  openvcc_yaml = templatefile("${path.module}/openvcc.yaml.tftpl", {
    aws_url      = "http://${module.app_aws.vm_public_ip}:8000"
    aws_region   = module.app_aws.region
    azure_url    = "http://${module.app_azure.vm_public_ip}:8000"
    azure_region = module.app_azure.region
  })
  engine_cloud_init = templatefile("${path.module}/cloud-init.engine.yaml.tftpl", {
    image        = var.engine_image
    admin_token  = var.admin_token
    openvcc_yaml = local.openvcc_yaml
  })
}

module "engine_aws" {
  source         = "../../modules/aws"
  name           = "${var.name}-engine-aws"
  region         = var.aws_region
  instance_size  = var.engine_instance_size
  ssh_public_key = var.ssh_public_key
  ingress_cidrs  = var.ingress_cidrs
  app_port       = 80 # proxy listens here in the container
  cloud_init     = local.engine_cloud_init
  tags           = { Stack = var.name, Role = "engine" }
}

module "engine_azure" {
  source         = "../../modules/azure"
  name           = "${var.name}-engine-azure"
  region         = var.azure_region
  instance_size  = var.azure_engine_instance_size
  ssh_public_key = var.ssh_public_key
  ingress_cidrs  = var.ingress_cidrs
  app_port       = 80
  cloud_init     = local.engine_cloud_init
  tags           = { Stack = var.name, Role = "engine" }
}
