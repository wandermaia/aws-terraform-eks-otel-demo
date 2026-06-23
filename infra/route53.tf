# Zona privada para testes
resource "aws_route53_zone" "private" {
  #name = "wandermaia.com"
  name = "wandermaia.com"

  vpc {
    vpc_id = module.vpc.vpc_id
  }
}

# CNAME que será associado com o RDS do mysql
resource "aws_route53_record" "dns-mysql" {
  zone_id = aws_route53_zone.private.zone_id
  name    = "mysql-${local.environment}.wandermaia.com"
  type    = "CNAME"
  ttl     = 300
  records = ["${aws_db_instance.db_mysql.address}"]
}
