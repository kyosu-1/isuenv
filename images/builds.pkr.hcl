build {
  sources = ["source.amazon-ebs.ubuntu"]

  provisioner "shell" {
    inline = ["cloud-init status --wait || true"]
  }

  provisioner "shell" {
    scripts = [
      "${path.root}/provision/common.sh",
      "${path.root}/provision/${var.problem}.sh",
    ]
  }

  # 公開AMIに残したくないものを消す。ビルド用の一時鍵は ssh_clear_authorized_keys が消す。
  provisioner "shell" {
    inline = [
      "sudo cloud-init clean --logs --seed",
      "sudo rm -rf /tmp/* /var/tmp/*",
      "sudo find /var/log -type f -exec truncate -s 0 {} +",
      "sudo rm -f /root/.bash_history /home/ubuntu/.bash_history /home/isucon/.bash_history",
      "sudo truncate -s 0 /etc/machine-id",
    ]
  }
}
