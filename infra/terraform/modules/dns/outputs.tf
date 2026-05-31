output "zone_id" {
  description = "Used by cert + ingress modules to create validation + A-ALIAS records."
  value       = aws_route53_zone.this.zone_id
}

output "nameservers" {
  description = "4 NS records to add at the registrar for the subdomain. Printed via local-exec during apply too."
  value       = aws_route53_zone.this.name_servers
}
