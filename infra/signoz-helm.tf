# Instalação do sigonz APM através do helm
resource "helm_release" "signoz" {

  depends_on = [
    null_resource.kubeconfig
  ]


  name             = "signoz"
  repository       = "https://charts.signoz.io"
  chart            = "signoz"
  namespace        = local.observability_namespace_k8s
  create_namespace = true
  version          = "0.116.2"

  # Ajustando os recursos de memória para o clickhouse, para evitar que o pod fique em crashloopbackoff por falta de memória.
  set {
    name  = "clickhouse.resources.requests.memory"
    value = "2Gi"
  }

  set {
    name  = "clickhouse.resources.limits.memory"
    value = "4Gi"
  }

}


# Instalação do opentelemetry no cluster EKS
resource "helm_release" "opentelemetry_eks" {

  depends_on = [
    helm_release.signoz
  ]


  name       = "k8s-infra"
  repository = "https://charts.signoz.io"
  chart      = "k8s-infra"
  namespace  = local.observability_namespace_k8s
  version    = "0.15.0"

  values = [
    file("./env/${local.environment}/K8s-Infra-value.yaml")
  ]

}


# Ingress  Signoz
resource "kubectl_manifest" "signoz_ingress" {
  yaml_body = <<-YAML
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  namespace: signoz
  name: ingress-signoz
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: '443'
    alb.ingress.kubernetes.io/certificate-arn: ${aws_acm_certificate.cert.arn}
spec:
  ingressClassName: alb-external
  rules:
    - http:
        paths:
        - path: /*
          pathType: ImplementationSpecific
          backend:
            service:
              name: signoz
              port:
                number: 8080

  YAML

  depends_on = [helm_release.signoz,
  aws_acm_certificate.cert]
}
