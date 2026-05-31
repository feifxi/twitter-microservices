resource "aws_db_subnet_group" "this" {
  name       = "${var.name}-aurora"
  subnet_ids = var.private_subnet_ids
  tags       = var.tags
}

resource "aws_security_group" "this" {
  name        = "${var.name}-aurora"
  description = "Aurora Postgres SV2 - ingress from in-VPC on 5432"
  vpc_id      = var.vpc_id
  tags        = var.tags
}

resource "aws_vpc_security_group_ingress_rule" "postgres_in" {
  security_group_id = aws_security_group.this.id
  cidr_ipv4         = var.vpc_cidr_block
  from_port         = 5432
  to_port           = 5432
  ip_protocol       = "tcp"
}

resource "aws_rds_cluster" "this" {
  cluster_identifier = "${var.name}-aurora"
  engine             = "aurora-postgresql"
  engine_mode        = "provisioned"
  engine_version     = var.engine_version
  database_name      = var.database_name
  master_username    = "twitter_admin"
  # RDS provisions + rotates the password and stores it in a Secrets Manager secret
  # named "rds!cluster-…". Chunk 5 reads that ARN via ExternalSecret.
  manage_master_user_password = true

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.this.id]

  serverlessv2_scaling_configuration {
    min_capacity             = var.min_capacity
    max_capacity             = var.max_capacity
    seconds_until_auto_pause = var.seconds_until_auto_pause
  }

  # Stage is recreated on demand; no point holding a final snapshot.
  skip_final_snapshot     = true
  backup_retention_period = 1
  apply_immediately       = true
  storage_encrypted       = true

  tags = var.tags
}

resource "aws_rds_cluster_instance" "writer" {
  identifier         = "${var.name}-aurora-writer"
  cluster_identifier = aws_rds_cluster.this.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.this.engine
  engine_version     = aws_rds_cluster.this.engine_version
  apply_immediately  = true
  tags               = var.tags
}
