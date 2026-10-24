#!/bin/bash
# 全問題のAMIに共通の下ごしらえ。問題ごとの provision/<問題名>.sh の前に実行する。
#
# 本番のメモリが 1GiB 前後の問題(isucon8-final など)では、Ubuntu 24.04 のクラウドイメージに常駐している
# 本番当時のVMには無かったデーモンだけで 150MB ほど使い、アプリが OOM で落ちる。競技に関係しないものを外す。
#   snapd + amazon-ssm-agent(snap)  約 55MB   isuenv は SSH で入るので SSM は使わない
#   multipathd                      約 27MB
#   unattended-upgrades             約 16MB   練習中に勝手にパッケージが更新されないようにする意味もある
#   networkd-dispatcher             約 16MB
#   udisks2                         約 13MB
#   ModemManager                    約 13MB
# 2026-10-24 に isucon8-final(mem=1G)で、2回目以降のベンチ中にアプリが OOM で kill されるのを確認して入れた。
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive

sudo snap remove --purge amazon-ssm-agent || true
sudo apt-get purge -y --auto-remove snapd unattended-upgrades multipath-tools networkd-dispatcher udisks2 modemmanager
sudo rm -rf /snap /var/snap /var/lib/snapd /root/snap /home/ubuntu/snap
# apt で snapd が戻ってこないようにする
sudo tee /etc/apt/preferences.d/isuenv-no-snapd >/dev/null <<'PREF'
Package: snapd
Pin: release a=*
Pin-Priority: -10
PREF
