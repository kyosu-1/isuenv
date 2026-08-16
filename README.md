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

- 環境はTTL経過後に**自動でterminate**される（絶対期限をcronで毎分チェックして`shutdown -P now` + terminate挙動。rebootされても期限は再評価されるので安全）
- SSHは `~/.ssh/isuenv_config` に自動生成される。素の `ssh isucon13-1` やVS Code Remoteも使える
- セキュリティグループは実行時のグローバルIPのみ許可。IPが変わったら `isuenv ssh` を打ち直せば更新される

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
