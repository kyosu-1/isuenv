locals {
  timestamp = formatdate("YYYYMMDDhhmmss", timestamp())
}

source "amazon-ebs" "ubuntu" {
  region          = var.region
  ami_name        = "isuenv-${var.problem}-${local.timestamp}"
  ami_description = "isuenv: ${var.problem} (https://github.com/kyosu-1/isuenv)"
  # ビルド直後は非公開。isuenv で立ててベンチが通るのを確かめてから images/publish.sh で公開する。
  ami_groups = []

  spot_instance_types = var.spot_instance_types
  spot_price          = "auto"

  source_ami_filter {
    filters = {
      name                = "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["099720109477"] # Canonical
  }

  launch_block_device_mappings {
    device_name           = "/dev/sda1"
    volume_type           = "gp3"
    volume_size           = var.volume_size
    delete_on_termination = true
  }

  ssh_username              = "ubuntu"
  ssh_interface             = "public_ip"
  ssh_clear_authorized_keys = true

  tags = {
    Name             = "isuenv-${var.problem}-${local.timestamp}"
    "isuenv:problem" = var.problem
  }
  run_tags = {
    Name = "isuenv-image-build-${var.problem}"
  }
}
