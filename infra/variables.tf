variable "environment" {
  description = "Define em qual ambiente estamos trabalhando (DEV ou PRD)"
  type        = string
  nullable    = false
}

variable "project_name" {
  description = "Define o nome do projeto"
  type        = string
  nullable    = false
}
variable "aws_region" {
  description = "Região onde será realizada a criação dos recursos"
  type        = string
  nullable    = false
}

variable "repositorio" {
  description = "Nome do repositório do projeto"
  type        = string
  nullable    = false
}

variable "vpc_cidr" {
  description = "CIDR que será utilizado para criar a VPC"
  type        = string
  nullable    = false
}

variable "eks_cluster_version" {
  description = "Versão do EKS que será utilizada na criação do cluster"
  type        = string
  nullable    = false
}
