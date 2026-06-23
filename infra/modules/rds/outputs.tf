output "db_endpoint" {
  description = "Endpoint de conexão do RDS"
  value       = aws_db_instance.mysql.endpoint
}

output "db_port" {
  description = "Porta do RDS"
  value       = aws_db_instance.mysql.port
}

output "security_group_id" {
  description = "ID do security group do RDS"
  value       = aws_security_group.mysql.id
}
