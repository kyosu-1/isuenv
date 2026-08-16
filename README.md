# isuenv

ISUCON過去問の練習環境（[matsuu/aws-isucon](https://github.com/matsuu/aws-isucon) の公開AMI）を
AWS EC2上にコマンド一発で構築・破棄するCLI。

## インストール

Homebrew（macOSのみ。Cask配布なのでLinuxbrewからは入らない）:

```sh
brew install kyosu-1/tap/isuenv
```

Go:

```sh
go install github.com/kyosu-1/isuenv@latest
```

[Releases](https://github.com/kyosu-1/isuenv/releases) から macOS / Linux（amd64・arm64）のバイナリを直接落としてもよい。

## 前提

- AWS認証情報（`AWS_PROFILE` などSDKの標準的な方法で解決される）
- リージョンは ap-northeast-1 固定
- EC2の vCPU クォータ（複数台構成を使う場合は6 vCPU以上）

## 使い方

```sh
isuenv problems               # 対応過去問一覧
isuenv up isucon13            # 環境作成（1台, TTL 8h, c5.large）
isuenv up isucon13 --nodes 3  # 本番同様の3台構成
isuenv list                   # 稼働中環境と概算コスト・残りTTL
isuenv ssh isucon13           # 1号機にSSH（isucon13-2 で2号機）
isuenv down isucon13          # 環境削除
isuenv nuke                   # isuenv管理の全リソース削除（VPC・キーペア含む）
isuenv version                # バージョン表示
```

## コマンドリファレンス

### `isuenv up <問題名> [flags]`

環境を作成し、全ノードがrunningかつパブリックIPが付くまで待ってから結果を表示する。

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `--ttl` | `8h` | この時間が経過したら自動でterminateする（[挙動](#ttlの挙動)） |
| `--nodes` | `1` | 起動台数。1以上 |
| `--instance-type` | `c5.large` | EC2インスタンスタイプ |

同名の環境が既にある場合は起動せずエラーになる。作り直すときは先に `down` する。

**`--ttl` の書式は Go の duration 文字列**で、単位は `h` / `m` / `s`。組み合わせもできる。

```sh
isuenv up isucon13 --ttl 90m      # 90分
isuenv up isucon13 --ttl 2h30m    # 2時間30分
```

**日を表す `d` は使えない。** `--ttl 1d` はエラーになるので、24時間なら `24h` と書く。

```
Error: invalid argument "1d" for "--ttl" flag: time: unknown unit "d" in duration "1d"
```

### `isuenv list`

稼働中の環境を一覧する。

| 列 | 内容 |
| --- | --- |
| `ENV` | 問題名 |
| `NODES` | 台数 |
| `TYPE` | インスタンスタイプ |
| `UPTIME` | 起動からの経過時間 |
| `EST COST` | 概算費用。あくまで目安 |
| `TTL LEFT` | 自動terminateまでの残り時間 |
| `PUBLIC IPS` | 各ノードのパブリックIP |

### `isuenv ssh <問題名>[-N]`

ノードにSSHする。番号を省略すると1号機に繋ぐ（`isucon13` = `isucon13-1`）。

実行のたびに次の2つを行うので、**グローバルIPが変わったら打ち直せば復旧する**。

- セキュリティグループのingressを、現在のグローバルIPで貼り直す
- `~/.ssh/isuenv_config` を稼働中の環境から再生成する

生成されたssh configは `~/.ssh/config` からIncludeされるので、素の `ssh isucon13-1` やVS Code Remoteからも使える。

### `isuenv down <問題名>`

その環境のインスタンスをterminateする。VPC・サブネット・SG・キーペアは残るので、次の `up` で再利用される。対象が無い場合も成功扱い。

### `isuenv nuke`

isuenv管理下の**全リソース**を削除する。`yes` の入力を求められる。インスタンスの終了を待ってから、キーペア・SG・サブネット・IGW・VPCの順に消す。

### `isuenv problems`

対応している過去問と、SSHユーザー、ベンチ手順へのリンクを一覧する。

## TTLの挙動

TTLの実体はインスタンス内の仕組みで、次の3段構えで動く。

1. 起動時のuser-dataが絶対期限（UNIX時刻）を `/var/lib/isuenv-expires-at` に書く
2. `/etc/cron.d/isuenv-ttl` が毎分その時刻を過ぎたか判定し、過ぎていれば `shutdown -P now`
3. インスタンスは `instance-initiated-shutdown-behavior=terminate` で起動しているため、停止ではなく**terminate**される（EBSごと消えるので課金が完全に止まる）

絶対時刻をディスクに持つので、**リブートしても期限は維持される**。判定が毎分なので、実際にterminateされるのは期限から1分程度あと。**ノートPCを閉じてもCLIを終了しても効く。**

## 作成されるリソース

AWS上のリソースはすべて `isuenv:managed=true` タグが付き、`nuke` の対象はこのタグで判定される。

| リソース | 内容 |
| --- | --- |
| VPC | `10.100.0.0/16` |
| サブネット | `10.100.0.0/24`（パブリックIP自動割当ON） |
| インターネットゲートウェイ | VPCにアタッチし、メインルートテーブルに `0.0.0.0/0` を向ける |
| セキュリティグループ | 名前 `isuenv`。**実行時のグローバルIP/32からのtcp 22/80/443** と、**自身のSGからの全プロトコル**（ノード間通信用） |
| キーペア | 名前 `isuenv` |
| EC2インスタンス | `--nodes` の台数 |

インスタンスには `isuenv:env`（問題名）、`isuenv:node`（何号機か）、`isuenv:expires-at`（TTLの絶対期限）のタグが付く。CLIはローカルに状態を持たず、すべてこれらのタグから復元する。

ローカルには次のファイルが作られる。

| パス | 内容 |
| --- | --- |
| `~/.ssh/isuenv.pem` | キーペアの秘密鍵（`0600`）。AWS側にキーペアが無いときに作成され、**このファイルも上書きされる**（`nuke` 後の `up` など） |
| `~/.ssh/isuenv_config` | ホスト定義。`up` / `ssh` / `down` のたびに再生成される |
| `~/.ssh/config` | 先頭に `Include` 行を一度だけ追加（パスは絶対パスで書かれる） |

VPC・サブネット・IGW・SG・キーペアは**無料**なので、`down` 後にこれらが残っていても費用は発生しない。

## コストの目安

c5.large（デフォルト）は約$0.107/時（ap-northeast-1、2026-07時点の概算）。
`isuenv list` の EST COST は概算であり、実際の課金はAWSの請求を確認すること。

## 手動E2E検証手順

コードを変更したら以下を実施する:

1. `go test ./... && go build -o isuenv .`
2. `./isuenv up isucon14 --ttl 1h`
3. `./isuenv list` — 環境が表示され、TTL LEFTが1h弱であること
4. `./isuenv ssh isucon14` — ログインできること。`sudo -i -u isucon` でアプリを確認
5. ブラウザで `http://<public ip>` にアクセスできること
6. `./isuenv down isucon14` — 削除されること
7. `./isuenv list` — 空になること
8. （まれに）`./isuenv nuke` でVPCまで消えることをAWSコンソールで確認
9. 複数台構成の疎通確認: `./isuenv up isucon13 --nodes 2` → `./isuenv ssh isucon13`（1号機）でログイン → `nc -zv <2号機のprivate ip> 22` が成功すること（SGの自己参照ルールでノード間通信が通ることの確認）→ `./isuenv down isucon13`

## リリース手順

`main` にタグを打つと GitHub Actions（`.github/workflows/release.yml`）が goreleaser を回し、
GitHub Releases へのバイナリ公開と [kyosu-1/homebrew-tap](https://github.com/kyosu-1/homebrew-tap) の
Formula 更新まで自動で行われる。

```sh
git switch main && git pull
git tag v0.1.0
git push origin v0.1.0
```

- タグは `vX.Y.Z` 形式のみ発火する（`v0.1.0-rc1` などは対象外）
- tap への push には `HOMEBREW_TAP_TOKEN` シークレット（homebrew-tap への `Contents: write` を持つPAT）が必要
- 設定を変更したらタグを打つ前に `goreleaser check` と `goreleaser release --snapshot --clean` で確認する（CIでも自動で検証される）

## 注意

- ベンチマーカーはAMIに同梱されている。実行方法は問題ごとに異なるので `isuenv problems` のNOTESのリンク先を参照
- 消し忘れてもTTLで自己消滅するが、`isuenv list` での確認を習慣にすること

## ライセンス

MIT License. 詳細は [LICENSE](LICENSE) を参照。

利用しているAMIは [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon)（MIT）が公開しているもので、
本ツールはそのAMIを起動・破棄するだけであり、AMIそのものは配布していない。
