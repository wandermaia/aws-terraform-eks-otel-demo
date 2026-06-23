# Coleta de dados essenciais para a configuração da infraestrutura, como as zonas de disponibilidade disponíveis na região e a identidade do usuário atual, 
# que são utilizados para definir as subnets e configurar o cluster EKS de forma adequada às condições do ambiente AWS onde será provisionado.
data "aws_availability_zones" "available" {}
data "aws_caller_identity" "current" {}

module "rds" {
  source = "./modules/rds"

  environment                 = local.environment
  rds_name                    = local.rds_name
  security_group_name         = local.rds_security_group_name
  vpc_id                      = module.vpc.vpc_id
  database_subnet_group_name  = module.vpc.database_subnet_group
  private_subnets_cidr_blocks = module.vpc.private_subnets_cidr_blocks
  database_username           = local.database_username
  database_password           = local.database_password
}

# Configurar kubeconfig do cluster criado
# Executa um comando na máquina local para configurar o acesso ao novo cluster criado.
resource "null_resource" "kubeconfig" {

  depends_on = [
    module.eks_blueprints_addons
  ]
  provisioner "local-exec" {
    command = "aws eks --region ${local.region} update-kubeconfig --name ${local.eks_cluster_name}"
  }
}
