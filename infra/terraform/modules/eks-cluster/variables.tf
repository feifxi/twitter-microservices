variable "name" {
  description = "EKS cluster name. Must match the kubernetes.io/cluster/<name> subnet tag set by the vpc module."
  type        = string
}

variable "cluster_version" {
  description = "Kubernetes minor version. EKS supports the last 4 minors."
  type        = string
  default     = "1.32"
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  description = "Subnets the control plane ENIs and node group are placed in."
  type        = list(string)
}

variable "node_instance_types" {
  type    = list(string)
  default = ["t3.medium"]
}

variable "node_min_size" {
  type    = number
  default = 2
}

variable "node_max_size" {
  type    = number
  default = 4
}

variable "node_desired_size" {
  type    = number
  default = 2
}

variable "tags" {
  type    = map(string)
  default = {}
}
