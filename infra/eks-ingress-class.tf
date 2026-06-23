# o AWS Load Balancer Controller já vem pré-instalado e é gerenciado automaticamente pela AWS.
# No entanto, para que ele funcione corretamente, é necessário realizar algumas configurações básicas de recursos do Kubernetes.
# https://docs.aws.amazon.com/eks/latest/userguide/auto-configure-alb.html

# Ingress Class Params alb interno
resource "kubectl_manifest" "ingress_class_internal_params" {
  yaml_body = <<-YAML
apiVersion: eks.amazonaws.com/v1
kind: IngressClassParams
metadata:
  name: alb-internal
spec:
  scheme: internal
  YAML

  depends_on = [
    kubectl_manifest.eks_node_pool,
    kubernetes_storage_class_v1.gp3
  ]
}


# Ingress Class Params alb interno
resource "kubectl_manifest" "ingress_class_external_params" {
  yaml_body = <<-YAML
apiVersion: eks.amazonaws.com/v1
kind: IngressClassParams
metadata:
  name: alb-external
spec:
  scheme: internet-facing
  YAML

  depends_on = [
    kubectl_manifest.eks_node_pool,
    kubernetes_storage_class_v1.gp3
  ]

}

# Ingress Class
resource "kubectl_manifest" "ingress_class_internal" {
  yaml_body = <<-YAML
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: alb-internal
  annotations:
    # Use this annotation to set an IngressClass as Default
    # If an Ingress doesn't specify a class, it will use the Default
    ingressclass.kubernetes.io/is-default-class: "true"
spec:
  # Configures the IngressClass to use EKS Auto Mode
  controller: eks.amazonaws.com/alb
  parameters:
    apiGroup: eks.amazonaws.com
    kind: IngressClassParams
    # Use the name of the IngressClassParams set in the previous step
    name: alb-internal
  YAML

  depends_on = [kubectl_manifest.ingress_class_internal_params]
}


# Ingress Class
resource "kubectl_manifest" "ingress_class_external" {
  yaml_body = <<-YAML
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: alb-external
  annotations:
    # Use this annotation to set an IngressClass as Default
    # If an Ingress doesn't specify a class, it will use the Default
    ingressclass.kubernetes.io/is-default-class: "true"
spec:
  # Configures the IngressClass to use EKS Auto Mode
  controller: eks.amazonaws.com/alb
  parameters:
    apiGroup: eks.amazonaws.com
    kind: IngressClassParams
    # Use the name of the IngressClassParams set in the previous step
    name: alb-external
  YAML

  depends_on = [kubectl_manifest.ingress_class_external_params]
}