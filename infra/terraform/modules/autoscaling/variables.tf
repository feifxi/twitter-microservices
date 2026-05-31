variable "namespace" {
  type = string
}

variable "hpa_targets" {
  description = "Services that get an HPA. Plan calls out tweet+feed only — the read/write hot path. Add more if a service starts gating throughput."
  type = map(object({
    min_replicas   = number
    max_replicas   = number
    cpu_target_pct = number
  }))
  default = {
    tweet-service = {
      min_replicas   = 1
      max_replicas   = 5
      cpu_target_pct = 60
    }
    feed-service = {
      min_replicas   = 1
      max_replicas   = 5
      cpu_target_pct = 60
    }
  }
}
