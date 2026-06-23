variable "environment" {
  description = "Ambiente de execução (ex: lab, prd)"
  type        = string
}

variable "rds_name" {
  description = "Nome do RDS"
  type        = string
}

variable "security_group_name" {
  description = "Nome do security group do RDS"
  type        = string
}

variable "vpc_id" {
  description = "ID da VPC onde o RDS será criado"
  type        = string
}

variable "database_subnet_group_name" {
  description = "Nome do subnet group de banco de dados"
  type        = string
}

variable "private_subnets_cidr_blocks" {
  description = "CIDRs das subnets privadas para liberar acesso ao MySQL"
  type        = list(string)
}

variable "database_username" {
  description = "Usuário master do banco de dados"
  type        = string
}

variable "database_password" {
  description = "Senha master do banco de dados"
  type        = string
  sensitive   = true
}
