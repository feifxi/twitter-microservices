output "service_name" {
  description = "ClusterIP service name. Used as KEYCLOAK_BASE_URL by services; routed publicly via the Ingress."
  value       = kubernetes_service.keycloak.metadata[0].name
}

output "namespace" {
  value = kubernetes_service.keycloak.metadata[0].namespace
}

output "internal_url" {
  description = "Cluster-internal URL. user-service uses this to call Keycloak admin API."
  value       = "http://${kubernetes_service.keycloak.metadata[0].name}.${kubernetes_service.keycloak.metadata[0].namespace}.svc.cluster.local:8080"
}

output "jwks_url" {
  description = "JWKS endpoint Kong validates JWTs against."
  value       = "http://${kubernetes_service.keycloak.metadata[0].name}.${kubernetes_service.keycloak.metadata[0].namespace}.svc.cluster.local:8080/realms/twitter/protocol/openid-connect/certs"
}

output "realm_public_key_pem" {
  description = "RSA public key (PEM) for the twitter realm. Kong's JWT plugin uses this as the consumer credential to validate tokens without hitting JWKS."
  value       = tls_private_key.realm.public_key_pem
}

output "realm_issuer" {
  description = "The 'iss' claim Keycloak puts in tokens — must match the public URL of the realm."
  value       = "https://${var.hostname}/realms/twitter"
}
