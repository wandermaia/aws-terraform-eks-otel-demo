resource "aws_security_group" "mysql" {
  name        = var.security_group_name
  description = "MySQL security group"

  vpc_id = var.vpc_id

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    cidr_blocks = var.private_subnets_cidr_blocks
    description = "Acesso ao MySQL das subnets privadas"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = var.security_group_name
  }
}

resource "aws_db_instance" "mysql" {
  identifier        = "rds-msql-${var.environment}"
  allocated_storage = 10
  db_name           = "mydb"
  engine            = "mysql"
  engine_version    = "8.0"
  instance_class    = "db.t4g.micro"
  username          = var.database_username
  password          = var.database_password

  vpc_security_group_ids = [aws_security_group.mysql.id]
  db_subnet_group_name   = var.database_subnet_group_name

  skip_final_snapshot = true

  tags = {
    Name = var.rds_name
  }
}
