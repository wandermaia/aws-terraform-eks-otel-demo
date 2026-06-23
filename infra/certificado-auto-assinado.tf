# Certificado autoassinado para uso no Load Balancer (HTTPS) - Para fins de teste e desenvolvimento local
# Gera uma chave privada RSA
resource "tls_private_key" "example" {
  algorithm = "RSA"
}

# Cria o certificado autoassinado (Válido por 30 dias para o teste)
resource "tls_self_signed_cert" "example" {
  private_key_pem = tls_private_key.example.private_key_pem

  subject {
    common_name  = "meu-lab-calculadora.local"
    organization = "Lab Corp"
  }

  validity_period_hours = 720 # 30 dias

  allowed_uses = [
    "key_encipherment",
    "digital_signature",
    "server_auth",
  ]
}

# Importa para o ACM (AWS Certificate Manager)
resource "aws_acm_certificate" "cert" {
  private_key      = tls_private_key.example.private_key_pem
  certificate_body = tls_self_signed_cert.example.cert_pem
}
