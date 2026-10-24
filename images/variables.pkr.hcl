variable "problem" {
  type        = string
  description = "問題名。provision/<problem>.sh を実行し、AMI名は isuenv-<problem>-<日時> になる"
}

variable "region" {
  type    = string
  default = "ap-northeast-1"
}

variable "volume_size" {
  type        = number
  default     = 20
  description = "ルートボリュームのサイズ(GiB)。AMIを使う全ノードがこのサイズ以上で起動する"
}

variable "spot_instance_types" {
  type    = list(string)
  default = ["c5.large", "c5a.large", "c6i.large", "c6a.large", "c7i.large", "c7a.large", "m5.large", "m6i.large"]
}
