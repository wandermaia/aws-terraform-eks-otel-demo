
# Blueprints disponibilizados pela AWS para utilitários importantes para o cluster
# https://registry.terraform.io/modules/aws-ia/eks-blueprints-addons/aws/latest
module "eks_blueprints_addons" {
  source  = "aws-ia/eks-blueprints-addons/aws"
  version = "~> 1.23.0"

  depends_on = [
    kubectl_manifest.ingress_class_internal
  ]

  cluster_name          = module.eks_cluster.cluster_name
  cluster_endpoint      = module.eks_cluster.cluster_endpoint
  cluster_version       = module.eks_cluster.cluster_version
  oidc_provider_arn     = module.eks_cluster.oidc_provider_arn
  enable_metrics_server = true

  # Tags do Cluster EKS
  tags = {
    Cluster = local.eks_cluster_name,
  }
}

