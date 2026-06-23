# No EKS Auto Mode, não é necessário o módulo de IRSA para o EBS CSI.
# A AWS gerencia as permissões automaticamente.

resource "kubernetes_storage_class_v1" "gp3" {

  depends_on = [module.eks_cluster]

  metadata {
    name = "sc-ebs-gp3-encrypted"
    annotations = {
      # Isso tornará este o StorageClass padrão do cluster
      "storageclass.kubernetes.io/is-default-class" = "true"
    }

  }

  storage_provisioner    = "ebs.csi.eks.amazonaws.com"
  volume_binding_mode    = "WaitForFirstConsumer"
  allow_volume_expansion = true

  allowed_topologies {
    match_label_expressions {
      key    = "eks.amazonaws.com/compute-type"
      values = ["auto"]
    }
  }

  parameters = {
    "type"      = "gp3"
    "encrypted" = "true"
  }
}
