# Security group adicional para o eks
resource "aws_security_group" "security_adicional_eks" {

  name        = "eks-adicional-${local.environment}-sg"
  description = "Aditional security group EKS"

  vpc_id = module.vpc.vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = module.vpc.public_subnets_cidr_blocks
    description = "Acesso ao eks"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Saida full"
  }

  tags = {
    Name = "eks-adicional-${local.environment}-sg"
  }
}
