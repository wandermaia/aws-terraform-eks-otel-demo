# Nodeclass padrão para criação do nodepool customizado.
resource "kubectl_manifest" "eks_node_class" {

  yaml_body = <<-YAML

apiVersion: eks.amazonaws.com/v1
kind: NodeClass
metadata:
  name: node-class-otel-demo
spec:
  role: ${aws_iam_role.eks_auto_node_role.name}
  subnetSelectorTerms:
    - tags:
        karpenter.sh/discovery: ${module.eks_cluster.cluster_name}
  securityGroupSelectorTerms:
    - id: "${module.eks_cluster.node_security_group_id}"
  # Configuração de disco
  ephemeralStorage:
    iops: 3000
    size: 50Gi
    throughput: 125
  # Inserção automática do map de tags formatado para YAML
  tags:
    Name: "eks-auto-node-otel-demo"
    Cluster: "${module.eks_cluster.cluster_name}"

  YAML

  depends_on = [
    module.eks_cluster
  ]

}


# Nodepool customizado usando o NodeClass criado acima.
resource "kubectl_manifest" "eks_node_pool" {

  yaml_body = <<-YAML

apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: nodepool-otel-demo
spec:
  template:
    spec:
      # O pool general-purpose tem peso 0 por padrão.
      # Definindo 100, este será sempre a primeira escolha.
      weight: 100 
      expireAfter: 168h
      nodeClassRef:
        group: eks.amazonaws.com
        kind: NodeClass
        name: node-class-otel-demo
      requirements:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
        - key: kubernetes.io/os
          operator: In
          values: ["linux"]
        - key: karpenter.sh/capacity-type # Padrão Auto Mode
          operator: In
          values: ["spot", "on-demand"]
        - key: eks.amazonaws.com/compute-type
          operator: In
          values: ["auto"]
        - key: eks.amazonaws.com/instance-category
          operator: In
          values: ["t", "c", "m", "r"]
        - key: "eks.amazonaws.com/instance-size"
          operator: In
          values: ["medium", "large", "xlarge", "2xlarge", "4xlarge"]
        - key: eks.amazonaws.com/instance-generation
          operator: Gt
          values: ["2"]
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 180s

  YAML

  depends_on = [kubectl_manifest.eks_node_class]
}
