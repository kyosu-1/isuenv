# isuenv — ISUCON練習環境CLI 設計ドキュメント

日付: 2026-07-08
ステータス: 承認済み

## 目的

ISUCONの過去問練習環境（過去問アプリ + ベンチマーカー入りサーバー）を、AWS EC2上にコマンド一発で構築・破棄できるGo製CLIを作る。matsuu/aws-isucon が公開している構築済みAMIを利用する。

## ゴール / 非ゴール

**ゴール**

- `isuenv up <問題名>` から数分でSSH可能な練習環境が手に入る
- 消し忘れによる課金事故が起きない（環境が自律的に自己消滅する）
- 本番同様の複数台構成（App/DB分離練習）を作れる
- ローカル状態ファイルなしで、どのマシンからでも環境を管理できる

**非ゴール（初期バージョンでは作らない）**

- ベンチ実行連携（`isuenv bench`）— 問題ごとにベンチ起動方法が違うため後回し。カタログのメモ欄に手動手順を記載するに留める
- Web UI
- AWS以外のクラウド / ローカルDocker対応
- AMIが存在しない過去問の cloud-init 自前構築

## CLIコマンド体系

```
isuenv up <problem> [--nodes N] [--ttl 8h] [--instance-type c5.large]
isuenv list
isuenv ssh <problem>[-N]
isuenv down <problem>
isuenv nuke
isuenv problems
```

- `up`: 環境作成。デフォルト1台・TTL 8時間・c5.large
- `list`: 起動中環境の一覧。各環境の経過時間・概算コスト(USD)・自動削除までの残り時間・各ノードのパブリック/プライベートIPを表示
- `ssh`: 対象環境へSSH接続。複数台構成では `isucon13-1` のようにノード番号で指定（省略時は1号機）
- `down`: 環境のEC2インスタンスを terminate
- `nuke`: isuenv管理タグが付いた全リソース（インスタンス・SG・キーペア・VPC等）を削除
- `problems`: 対応過去問カタログを表示

## 実装方式

- 言語: Go（シングルバイナリ、外部ツール依存なし）
- AWS操作: AWS SDK for Go v2 を直接利用（Terraform/CloudFormationは使わない）
- CLIフレームワーク: cobra

## AWSリソース設計

- リージョン: `ap-northeast-1` 固定（matsuu AMIの公開先）
- インスタンスタイプ: デフォルト `c5.large`（本番レギュレーション相当）、`--instance-type` で変更可
- ネットワーク: 初回 `up` 時に isuenv 専用の VPC / パブリックサブネット×1 / インターネットゲートウェイ / ルートテーブル / セキュリティグループを作成し、以後使い回す
- セキュリティグループ: 実行時の自分のグローバルIP（例: `x.x.x.x/32`）のみ 22/80/443 を許可。`up` / `ssh` 実行時にIPが変わっていたらルールを自動更新。加えて、SG自身からの全プロトコルを許可する自己参照ルールを常に維持し、`--nodes N` 構成のノード間（プライベートIP経由）通信を可能にする
- 複数台構成: `--nodes N` で同一AMIをN台、同一サブネットに起動。ノード名は `<problem>-1`, `<problem>-2`, ...

## 過去問カタログ

- バイナリ埋め込み（`go:embed`）のYAMLで管理
- エントリ項目: 問題名 / AMI名パターン / AMI所有者アカウントID / 推奨ノード数 / メモ（ベンチの起動方法・初期パスワード等）
- AMI IDは固定せず、`DescribeImages`（owner指定 + 名前パターン）で最新AMIを実行時に解決する。matsuu氏のAMI更新に自動追従できる
- 新しい問題への対応はカタログにエントリを1つ足すだけ

## 消し忘れ防止

- `up` 時にuser-dataで絶対期限（`Now + TTL` のUnixエポック秒）を `/var/lib/isuenv-expires-at` に書き込み、`/etc/cron.d/isuenv-ttl` で毎分「現在時刻がその期限を過ぎたか」をチェックして超えていれば `shutdown -P now` する仕組みを仕込み、`instance-initiated-shutdown-behavior=terminate` を設定する。CLIやローカルマシンが死んでいても、EC2がTTL経過後に自力でterminateされる。相対時間指定の `shutdown -P +N` はreboot時にキャンセルされ、かつuser-dataは初回起動時にしか実行されないためreboot後は永久に動き続けてしまう。このcronベースの絶対期限方式はreboot後も再評価されるためreboot-safe
- 「停止」ではなく「削除」に倒す。AMIからいつでも作り直せるため、EBS課金を残す停止に意味はない
- `list` で経過時間・概算コスト・残りTTLを常に可視化
- `nuke` で全掃除

## 状態管理

ローカル状態ファイルを持たない。全リソースにタグを付け、照会はすべてAWSへの問い合わせで行う:

- `isuenv:managed = true`
- `isuenv:env = <問題名>`
- `isuenv:node = <ノード番号>`
- `isuenv:expires-at = <RFC3339時刻>`

複数マシンから使っても矛盾せず、状態ファイル破損の心配がない。

## SSH支援

- 初回に isuenv 専用キーペアを作成し、秘密鍵を `~/.ssh/isuenv.pem`（0600）に保存
- `~/.ssh/config` に `Include ~/.ssh/isuenv_config` を一度だけ追記（既にあれば何もしない）
- `up` / `down` のたびに `~/.ssh/isuenv_config` を再生成し、`Host isucon13-1` 形式のエントリを列挙 → 素の `ssh` や VS Code Remote からも接続できる

## エラーハンドリング方針

- `up` の途中失敗時は、作成済みリソースをタグで特定してロールバック（terminate）してから終了する
- AMIが見つからない・クォータ超過などAWS側のエラーは、原因と対処（例: vCPUクォータ申請）をメッセージで案内する
- `down` / `nuke` は冪等（対象がなければ成功扱い）

## テスト方針

- AWS SDK呼び出しはインターフェースに切り出し、ユニットテストはモックで実施（カタログ解決・タグフィルタ・TTL計算・ssh_config生成など）
- 実環境E2E（`up → ssh → down`）は手動。手順をREADMEに記載

## リポジトリ

`/Users/abe/ghq/github.com/kyosu-1/isuenv`（GitHub: kyosu-1/isuenv 想定）
