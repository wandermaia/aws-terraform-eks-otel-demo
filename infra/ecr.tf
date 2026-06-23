# Criação de repositórios ECR para armazenar as imagens Docker do backend e frontend da aplicação.
resource "aws_ecr_repository" "this" {
  for_each = toset(local.ecr_repositories)

  name                 = each.key
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}
