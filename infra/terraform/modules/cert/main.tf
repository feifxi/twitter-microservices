resource "aws_acm_certificate" "this" {
  domain_name = var.domain
  # Wildcard for api.<domain> + auth.<domain> + any future subdomain.
  subject_alternative_names = ["*.${var.domain}"]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }

  tags = var.tags
}

# DNS-01 validation records, one per FQDN on the cert (apex + wildcard).
resource "aws_route53_record" "validation" {
  for_each = {
    for d in aws_acm_certificate.this.domain_validation_options : d.domain_name => {
      name   = d.resource_record_name
      type   = d.resource_record_type
      record = d.resource_record_value
    }
  }

  zone_id         = var.zone_id
  name            = each.value.name
  type            = each.value.type
  records         = [each.value.record]
  ttl             = 60
  allow_overwrite = true
}

# Blocks the apply until ACM marks the cert ISSUED. ALB listener can't reference
# a PENDING_VALIDATION cert, so the downstream Ingress would fail without this gate.
# Hangs until DNS propagates - if you haven't added NS records at the registrar
# yet, terraform waits here (up to 75min default timeout).
resource "aws_acm_certificate_validation" "this" {
  certificate_arn         = aws_acm_certificate.this.arn
  validation_record_fqdns = [for r in aws_route53_record.validation : r.fqdn]
}
