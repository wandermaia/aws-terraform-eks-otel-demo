# RDS que será utilizado pela aplicação
resource "aws_db_instance" "db_mysql" {
  identifier        = "rds-msql-${var.environment}"
  allocated_storage = 10
  db_name           = "mydb"
  engine            = "mysql"
  engine_version    = "8.0"
  instance_class    = "db.t4g.micro"
  username          = local.database_username
  password          = local.database_password

  vpc_security_group_ids = [aws_security_group.security_group_mysql.id]
  db_subnet_group_name   = module.vpc.database_subnet_group

  skip_final_snapshot = true


  # Tags do RDS
  tags = {
    Name = local.rds_name
  }

}

