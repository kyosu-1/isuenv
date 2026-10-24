#!/bin/sh
# 古い世代のAMIを登録解除し、スナップショットも消す。使い方: images/cleanup.sh <問題名> [残す世代数(既定 1)]
#
# スナップショットの保管料は世代数ぶん積み上がるので、作り直したら古いものを消す。
# 新しい順に keep 個を残す。起動済みのインスタンスには影響しない。
set -eu
problem=${1:?usage: $0 <problem> [keep]}
keep=${2:-1}
region=${AWS_REGION:-ap-northeast-1}
aws ec2 describe-images --region "$region" --owners self \
  --filters "Name=name,Values=isuenv-${problem}-*" \
  --query 'reverse(sort_by(Images,&CreationDate))[].[ImageId,Name,BlockDeviceMappings[0].Ebs.SnapshotId]' --output text |
  tail -n +"$((keep + 1))" |
  while read -r ami name snap; do
    echo "deregister $ami ($name), delete $snap"
    aws ec2 deregister-image --region "$region" --image-id "$ami"
    aws ec2 delete-snapshot --region "$region" --snapshot-id "$snap"
  done
