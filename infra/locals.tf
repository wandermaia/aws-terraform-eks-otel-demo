locals {
  project_name                = var.project_name
  region                      = var.aws_region
  vpc_cidr_block              = var.vpc_cidr
  account_id                  = data.aws_caller_identity.current.account_id
  repositorio                 = var.repositorio
  environment                 = var.environment
  observability_namespace_k8s = "signoz"

  # Seleciona as primeiras 2 zonas de disponibilidade (AZs) da região para distribuir recursos e garantir alta disponibilidade
  azs_to_use = slice(data.aws_availability_zones.available.names, 0, 2)

  # Dados para a criação do cluster EKS
  eks_cluster_name        = "eks-${var.project_name}"
  eks_version             = var.eks_cluster_version
  eks_auto_node_role_name = "eks-auto-node-role-${var.project_name}"

  # Lista de repositórios ECR a serem criados.
  ecr_repositories = [
    "calculadora-backend",
    "calculadora-frontend",
    "joke-factor",
  ]

  # Dados para a criação do RDS MySQL
  rds_name                = "rds-msql-${local.project_name}"
  rds_security_group_name = "security-group-${local.rds_name}"
  database_name           = "demodb"
  database_username       = "admin_user"
  database_password       = "SenhaSegura123!" # Senha inicial. Deve ser alterada assim que possível por questões de segurança.


  # TAGs padrão para todos os recursos, facilitando a identificação e organização na AWS
  common_tags = {
    Project     = local.project_name
    Repository  = local.repositorio
    Environment = local.environment
    ManagedBy   = "Terraform"
    Analista    = "Wander Maia"
    aws-apn-id  = "pc:1dj5fgj8arumg9flsx6tqw5bc"
  }
}
