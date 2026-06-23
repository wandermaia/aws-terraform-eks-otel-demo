module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.6.0"

  name = "vpc-${local.project_name}"

  # Define o bloco CIDR da VPC com 65.536 endereços IP disponíveis
  cidr = var.vpc_cidr # vpc_cidr = "10.45.0.0/16"

  # Usa as 2 primeiras zonas de disponibilidade da região para distribuir recursos
  azs = local.azs_to_use

  # Função cidrsubnet divide o CIDR principal em subnets menores:
  # - prefix: CIDR base (10.45.0.0/16)
  # - newbits: 6 bits adicionais (/16 + 6 = /22, gerando 1.024 IPs por subnet)
  # - netnum: índice sequencial para cada subnet

  # Database Subnets (/24): Índices 0, 1 (10.45.0.0/24, 10.45.1.0/24)
  # Usamos 8 bits adicionais (16+8=24)
  database_subnets = [for k, v in local.azs_to_use : cidrsubnet(local.vpc_cidr_block, 8, k)]

  # Private Subnets (/22): Índices 1, 2 (10.45.4.0/22, 10.45.8.0/22)
  # Usamos 6 bits adicionais (16+6=22). 
  # O netnum '1' para /22 começa em 10.45.4.0, deixando os primeiros 10.45.0.0 até 10.45.3.255 livres para as /24.
  private_subnets = [for k, v in local.azs_to_use : cidrsubnet(local.vpc_cidr_block, 6, k + 1)]

  # Public Subnets (/24): Índices 48, 49 (10.45.48.0/24, 10.45.49.0/24)
  # Usamos 8 bits adicionais (16+8=24).
  # Escolhi o índice 48 para ficarem bem distantes das privadas e evitar qualquer overlap.
  public_subnets = [for k, v in local.azs_to_use : cidrsubnet(local.vpc_cidr_block, 8, k + 48)]


  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true
  enable_dns_support   = true

  # Tags gerais
  tags = {
    Name = module.vpc.name
  }

  database_subnet_group_name = "dabase-subnet-group-${var.environment}"

  # Tags das subnets para EKS
  private_subnet_tags = {
    Name                                              = "private-subnet-${module.vpc.name}"
    "kubernetes.io/role/internal-elb"                 = "1" # Necessário para AWS Load Balancer Controller identificar subnets privadas
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "shared"
    "karpenter.sh/discovery"                          = local.eks_cluster_name # Necessário para Karpenter descobrir as subnets privadas
  }

  public_subnet_tags = {
    Name                                              = "public-subnet-${module.vpc.name}"
    "kubernetes.io/role/elb"                          = "1" # Necessário para AWS Load Balancer Controller identificar subnets publicas
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "shared"
  }
  database_subnet_tags = {
    Name = "database-subnet-${module.vpc.name}"
  }

  private_route_table_tags = {
    Name = "private-route-table-${module.vpc.name}"
  }

  public_route_table_tags = {
    Name = "public-route-table-${module.vpc.name}"
  }


  igw_tags = {
    Name = "internet-gateway-${module.vpc.name}"
  }

  nat_gateway_tags = {
    Name = "nat-gateway-${module.vpc.name}"
  }

}
