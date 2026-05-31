resource "aws_route53_zone" "this" {
  name = var.domain
  tags = var.tags
}

# Prints the 4 nameservers to apply output the moment the zone is created.
# Copy these to Namecheap (Advanced DNS > Add NS records for subdomain) so the
# downstream ACM cert validation has DNS to resolve against. Without this, the
# cert validation step further down will hang for up to 75 min.
resource "null_resource" "print_nameservers" {
  triggers = {
    zone_id = aws_route53_zone.this.id
  }

  provisioner "local-exec" {
    interpreter = ["/bin/sh", "-c"]
    command     = <<-EOT
      echo ""
      echo "================================================================================"
      echo "  Route53 zone for ${var.domain} created."
      echo "  ADD THESE 4 NS RECORDS AT YOUR REGISTRAR FOR SUBDOMAIN '${split(".", var.domain)[0]}':"
      echo "${join("\n", [for ns in aws_route53_zone.this.name_servers : "    ${ns}"])}"
      echo ""
      echo "  The ACM cert validation downstream will hang until DNS propagates (~5 min after"
      echo "  records are saved). You can let terraform keep running while you add them."
      echo "================================================================================"
      echo ""
    EOT
  }
}
