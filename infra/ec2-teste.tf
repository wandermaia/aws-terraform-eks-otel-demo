# Instancia para API de teste
resource "aws_instance" "api_server" {

  instance_type = "t3a.micro"
  subnet_id     = element(module.vpc.private_subnets, 0)
  ami           = "ami-096f5760b00bcd95c" # Ubuntu 24.04 - us-west-2 oregon
  key_name      = "teste-ubuntu-oregon"

  # security_groups = [aws_security_group.security_group_ec2.id]
  # Em uma VPC, a AWS recomenda usar vpc_security_group_ids (que espera uma lista de IDs). 
  # O argumento security_groups costuma causar "drift" (diferença) porque o Terraform tenta converter o nome para ID 
  # e vice-versa a cada execução, forçando a recriação.

  vpc_security_group_ids = [aws_security_group.security_group_ec2.id]

  # Instance profile criado para acesso ao SSM
  iam_instance_profile = aws_iam_instance_profile.ec2_profile.name



  tags = {
    Name                         = "${local.name_prefix}-ec2-ubuntu"
    "SCH_AWSBACKUP_DAILY_23H-NV" = "SIM"
  }
}


# Security group para Instancia de API
resource "aws_security_group" "security_group_ec2" {

  name        = "${local.name_prefix}-ec2-sg"
  description = "EC2 security group"

  vpc_id = module.vpc.vpc_id

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Acesso completo"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Saida do servidor"
  }

  tags = {
    Name = "${local.name_prefix}-ec2-sg"
  }
}


# 1. Cria a IAM Role para a EC2
resource "aws_iam_role" "ec2_role" {
  name = "${local.name_prefix}-ec2-ssm-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })
}

# 2. Anexa a Policy do SSM na Role
resource "aws_iam_role_policy_attachment" "ssm_policy" {
  role       = aws_iam_role.ec2_role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# 3. Cria o Instance Profile que a EC2 de fato utiliza
resource "aws_iam_instance_profile" "ec2_profile" {
  name = "${local.name_prefix}-ec2-instance-profile"
  role = aws_iam_role.ec2_role.name
}

