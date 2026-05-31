resource "aws_security_group" "this" {
  name        = "${var.name}-msk"
  description = "MSK brokers - ingress from in-VPC on 9092 (plaintext) and 9094 (TLS)"
  vpc_id      = var.vpc_id
  tags        = var.tags
}

resource "aws_vpc_security_group_ingress_rule" "kafka_plain" {
  security_group_id = aws_security_group.this.id
  cidr_ipv4         = var.vpc_cidr_block
  from_port         = 9092
  to_port           = 9092
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "kafka_tls" {
  security_group_id = aws_security_group.this.id
  cidr_ipv4         = var.vpc_cidr_block
  from_port         = 9094
  to_port           = 9094
  ip_protocol       = "tcp"
}

resource "aws_msk_cluster" "this" {
  cluster_name           = "${var.name}-msk"
  kafka_version          = var.kafka_version
  number_of_broker_nodes = var.broker_count

  broker_node_group_info {
    instance_type = var.instance_type
    # MSK constraints: client_subnets must be 2 or 3, AND broker_count must be
    # a multiple of len(client_subnets). With 2 subnets, broker_count must be 2,4,6...
    client_subnets  = slice(var.private_subnet_ids, 0, var.broker_count)
    security_groups = [aws_security_group.this.id]

    storage_info {
      ebs_storage_info {
        volume_size = var.ebs_volume_size_gb
      }
    }
  }

  # TLS_PLAINTEXT lets the existing Go services connect plain in-VPC while still
  # exposing a TLS listener if we later harden the client config.
  encryption_info {
    encryption_in_transit {
      client_broker = "TLS_PLAINTEXT"
      in_cluster    = true
    }
  }

  tags = var.tags
}
