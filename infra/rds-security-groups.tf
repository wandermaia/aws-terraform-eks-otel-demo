# security group para mysql
resource "aws_security_group" "security_group_mysql" {

  name        = local.rds_security_group_name
  description = "MySQL security group"

  vpc_id = module.vpc.vpc_id

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    cidr_blocks = module.vpc.private_subnets_cidr_blocks
    description = "Acesso ao MySQL das subnets privadas"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Tags do RDS
  tags = {
    Name = local.rds_security_group_name
  }

}

