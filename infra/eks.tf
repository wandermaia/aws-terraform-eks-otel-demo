
module "eks_cluster" {
  source                  = "terraform-aws-modules/eks/aws"
  version                 = "~> 21.15.1"
  name                    = local.eks_cluster_name
  kubernetes_version      = local.eks_version
  vpc_id                  = module.vpc.vpc_id
  subnet_ids              = module.vpc.private_subnets
  endpoint_private_access = false
  endpoint_public_access  = true

  # Security group adicional para permitir comunicações
  additional_security_group_ids = [aws_security_group.security_adicional_eks.id]

  # Habilita o suporte a Access Entries
  authentication_mode = "API_AND_CONFIG_MAP"

  # Determina se deve criar um OpenID Connect Provider para EKS para habilitar IRSA
  # Habilitando funções do IAM para contas de serviço.
  enable_irsa = true

  # Forçar a criação dos recursos de IAM para o EKS Auto Mode. Esses recursos são necessários para criar os nós do nodepool customizado.
  create_auto_mode_iam_resources = true

  # Habilita o EKS Auto Mode
  compute_config = {
    enabled    = true
    node_pools = null # Necessário para criar um nodpool customizado.
    # node_pools = ["general-purpose"] # Cria automaticamente o pool padrão
  }

  # Configura as permissões para o EKS Auto Mode criar os nós do nodepool customizado usando a role do IAM criada.
  access_entries = {
    custom_node_role = {
      principal_arn = aws_iam_role.eks_auto_node_role.arn
      type          = "EC2"

      policy_associations = {
        auto_node = {
          policy_arn   = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSAutoNodePolicy"
          access_scope = { type = "cluster" }
        }
      }
    }
  }

  # O Auto Mode gerencia add-ons essenciais automaticamente
  # mas você ainda pode habilitar permissões de admin para si mesmo
  enable_cluster_creator_admin_permissions = true

  # Tags do Cluster EKS
  tags = {
    Name        = local.eks_cluster_name,
    EKS-Version = local.eks_version
  }

}
