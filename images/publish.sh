#!/bin/sh
# 検証済みのAMIを公開する。使い方: images/publish.sh <ami-id>
#
# 公開するのは isuenv で立ててベンチが通るのを確かめたAMIだけにする。isuenv は名前パターンに合う
# 最新のAMIを選ぶので、公開した時点から全利用者の `isuenv up` がこのAMIを使う。
#
# アカウントで「Block public access for AMIs」が有効だと公開できない。初回だけ次で無効にする:
#   aws ec2 disable-image-block-public-access --region ap-northeast-1
set -eu
ami=${1:?usage: $0 <ami-id>}
region=${AWS_REGION:-ap-northeast-1}
aws ec2 describe-images --region "$region" --image-ids "$ami" --owners self --query 'Images[0].Name' --output text
aws ec2 modify-image-attribute --region "$region" --image-id "$ami" --launch-permission 'Add=[{Group=all}]'
aws ec2 describe-images --region "$region" --image-ids "$ami" --query 'Images[0].[ImageId,Name,Public]' --output text
