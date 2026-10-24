# images

上流（matsuu/aws-isucon）にAMIが無い問題のAMIを作る Packer 定義。今は isucon6-final・isucon7-final・isucon8-final。
作ったAMIは isuenv のメンテナのAWSアカウント（`738925651667`）から公開し、`internal/catalog/catalog.yaml` の
`ami_pattern: "isuenv-<問題名>-*"` / `owner_id: "738925651667"` で引く。

| ファイル | 内容 |
| --- | --- |
| `sources.pkr.hcl` / `builds.pkr.hcl` / `variables.pkr.hcl` / `plugins.pkr.hcl` | 全問題で共通の Packer 定義。Ubuntu 24.04 をスポットで起動して provision を流す |
| `provision/common.sh` | 全問題に共通の下ごしらえ（メモリを食う常駐プロセスを外す） |
| `provision/<問題名>.sh` | 問題ごとの構築。本番との違いと、当時のままでは動かない箇所の直し方をコメントに書いてある |
| `publish.sh` | 検証済みのAMIを公開する |
| `cleanup.sh` | 古い世代のAMIを登録解除し、スナップショットを消す |

## 作り直す手順

AWSの認証情報はメンテナの個人アカウントのもの（SDKから使えるプロファイル）を使う。

```sh
cd images
packer init .
AWS_PROFILE=personal-sdk packer build -var problem=isucon8-final .   # 15〜20分。最後に AMI ID が出る
```

1. ビルド直後のAMIは**非公開**。isuenv は「名前パターンに合う最新のAMI」を選ぶので、この時点で `isuenv up` すると
   自分のアカウントからだけ新しいAMIが使われる。これで公開前に検証できる
2. 本番の構成で立ててベンチを回す（例: `isuenv up isucon8-final --nodes 4 --bench` → `$(isuenv bench isucon8-final)`）。
   再起動して初期状態に戻ることも確かめる。結果は catalog.yaml のコメントに日付付きで残す
3. 公開する: `AWS_PROFILE=personal ./publish.sh <ami-id>`
   （アカウントの「Block public access for AMIs」が有効なら、初回だけ `aws ec2 disable-image-block-public-access --region ap-northeast-1`）
4. 古い世代を消す: `AWS_PROFILE=personal ./cleanup.sh isucon8-final`（新しい1世代だけ残す）
5. 作り直した・消したことをトップの README で告知する

## 費用

AMIの保管料（スナップショット）は $0.05/GB・月。3問で月 $1〜2 程度。ほかの人が起動したときの EC2 代はその人の負担になる。
ビルドはスポットインスタンスで行うので1回数セント。検証で本番の構成を立てるのが一番かかる（isucon8-final の5台で約 $0.88/時）。
