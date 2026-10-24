# isucon6〜8 本戦対応（自前AMI）設計ドキュメント

日付: 2026-10-24
ステータス: 実装済み(2026-10-24)

## 目的

isucon6-final / isucon7-final / isucon8-final を isuenv で立てられるようにする。

isuenv は「公開AMIを名前パターンで解決して起動する」CLIだが、この3問は公開AMIが存在しない
(matsuu/aws-isucon・matsuu/vagrant-isucon に無く、2026-10-24 に ap-northeast-1 の公開AMIを
名前で検索しても見つからない)。そこで **isuenv 自身が Packer でAMIを作り、メンテナの個人アカウントから公開する**。
起動は他の問題と同じく数分で終わることを優先し、起動時に user-data で一から構築する方式は採らない
(8本戦の試算で起動後 10 分前後かかり、古い Dockerfile の取得元消失などで毎回壊れうるため)。

## ゴール / 非ゴール / アンチゴール

**ゴール**

- `isuenv up isucon8-final` などで、他の問題と同じ手触り(数分で SSH 可能、AMI名を表示、TTLで自己消滅)で立つ
- 本番の台数(`--nodes`)・公式スペック(CPU/メモリ制限)・ベンチノード(`--bench`)を既存の仕組みどおり再現する
- 各問題で Go 実装がベンチを pass することを実機で確認してから AMI を公開する
- AMI を作り直せる(Packer 定義と provision スクリプトがリポジトリにある)

**非ゴール**

- matsuu/aws-isucon への還元(書式を合わせる必要はない)
- Go 以外の言語実装の動作保証。7本戦は Go だけ入れる。6本戦・8本戦はリポジトリ全体を置くが、確認するのは Go のみ
- 本番の NIC 分離(グローバル/プライベート/ベンチ用)と帯域制限の再現。プライベートIP 1本で通信する
- 本番のポータル・consul・proxy(6本戦)の再現。ベンチは CLI で直接対象を叩く
- CI での AMI 自動ビルド。ビルドは手元から手動
- 起動時に user-data で一から構築する方式

**アンチゴール(動いても失敗とみなすもの)**

- ベンチが通るか確認していない AMI が公開され、`isuenv up` の「最新AMI」として選ばれる状態
- 公開AMIに SSH 鍵・ビルド時の認証情報・シェル履歴が残っている
- 既存の問題の挙動が変わる
- 公開情報で確認できない本番スペックを推測で埋める(catalog.yaml の既存方針どおり「非公開」と書く)

## 決まったこと

| 項目 | 決定 |
| --- | --- |
| イメージの用意 | 自前の Packer で AMI をビルドし、公開AMIにする |
| 費用 | 3問で保管料 月 $1.5〜2 程度(isucon14 の自前AMIの実測スナップショット 12.8GiB を目安に推定)を許容 |
| ベースOS | 3問とも Ubuntu 24.04 (Canonical 公式AMI, owner 099720109477) |
| 7本戦の言語 | Go だけ |
| 上流への還元 | 考えない |

## 構成

### Packer 定義の置き場所と形

isuenv リポジトリに同梱し、1つの Packer 定義を問題名で切り替える(問題ごとの差は provision スクリプトに閉じる)。

```
images/
  sources.pkr.hcl      # amazon-ebs。Ubuntu 24.04 amd64 を source_ami_filter で解決。スポットでビルド
  builds.pkr.hcl       # cloud-init 完了待ち → provision/${var.problem}.sh → 後始末
  variables.pkr.hcl    # problem, volume_size, public(既定 false)
  provision/
    isucon8-final.sh
    isucon6-final.sh
    isucon7-final.sh
  publish.sh           # 検証済みAMIを公開する(launch permission に all を足す)
  cleanup.sh           # 古い世代のAMIの登録解除とスナップショット削除
  README.md            # ビルド・検証・公開・掃除の手順
```

- ビルド: `AWS_PROFILE=personal-sdk packer build -var problem=isucon8-final images/`
- AMI 名: `isuenv-<問題名>-<YYYYMMDDhhmmss>`(matsuu の AMI と区別するため `isuenv-` を付ける)
- 所有アカウント: 738925651667(メンテナの個人アカウント)。catalog の `owner_id` にこれを書く
- ビルド直後は非公開。isuenv で立ててベンチ pass を確認してから `publish.sh` で公開する
  (isuenv は「パターンに合う最新の available なAMI」を選ぶので、非公開のうちはメンテナ以外には見えず、
  メンテナ自身の `isuenv up` だけが新しいAMIを掴む。これで公開前の検証ができる)
- アカウントの「Block public access for AMIs」は現在有効(`block-new-sharing`)。初回公開前に一度だけ手動で無効化する
- 後始末: ビルド用の一時鍵は `ssh_clear_authorized_keys` で消す。シェル履歴・cloud-init のログやインスタンス固有の状態・
  apt キャッシュも消す
- 作り直したら `cleanup.sh` で古い世代を消す(保管料が世代数ぶん積み上がるため)。直前の1世代は残すかは実装者の判断

### AMI の中身の共通方針

- `ubuntu` ユーザーで SSH(catalog の `ssh_user: ubuntu`)。アプリとベンチは `isucon` ユーザーの持ち物にし、
  `isucon` は sudo NOPASSWD(他の問題のAMIと同じ慣習)
- 初期状態(どのサービスが動いているか)は本番の初期状態に合わせる。systemd で自動起動し、再起動後も同じ状態に戻る
- 古い Dockerfile・依存取得が今の環境で壊れている箇所は provision スクリプトで直す。直した箇所は
  スクリプトにコメントで理由を残す(catalog.yaml と同じく「何が壊れていて、実機でどう確認したか」)
- メモリ制限は AMI に焼き込まない。ビルドはメモリを絞らずに行い、制限は従来どおり isuenv の user-data が起動時にかける
- 初期データの投入はビルド時に済ませておく(起動時間を他の問題並みにするため)

### isuenv 側の変更

1. **catalog.yaml に3問を追加**。既存の書式・方針どおり、スペックの根拠を値ごとにコメントで残す
2. **ノード番号と台数を AMI に渡す汎用の仕組み**(6本戦・7本戦で必要)
   - user-data で `/etc/isuenv/node.env` のようなファイルにノード番号・台数(と後述の他ノードのIP)を書き、
     AMI 側に `/usr/local/bin/isuenv-node-init` のような初期化スクリプトがあれば実行する。
     問題固有の処理は AMI 側に置き、isuenv は事実を渡すだけにする。名前・形式は実装者の判断
   - 初期化スクリプトが無い AMI(既存の全問題)では何も起きないこと
   - メモリ制限の再起動と順序が絡む(再起動後に user-data が再実行されうる)ので、初期化は冪等にする
3. **他ノードのプライベートIPを起動時に知る仕組み**(7本戦で必要)
   - 制約: `--name` で同じ問題の環境を並べて立てても衝突しないこと。isuenv の VPC/サブネットは全環境で共有
   - 案(推奨): RunInstances でプライベートIPを明示指定する。サブネット内の空きIPを選び、
     使用中エラーなら選び直す。全ノードのIPが起動前に決まるので user-data に書ける
   - 別案: DB ノードを先に起動して IP を得てから残りを起動する(2段階起動)
   - どちらにするかは実装者の判断。7本戦に着手するときに決めればよい
4. **README**: 対応問題の表に3問を追加。これらの AMI は isuenv のメンテナが自分のアカウントで公開していること、
   作り直し・削除時は README で告知することを書く

## 問題ごとの要件

各問題の本番スペックは、公式リポジトリ(isucon/isucon{6,7,8}-final)の README・マニュアル・provisioning と
公式ブログから取る。見つからない値は「非公開」として埋めない。

### isucon8-final(最初に対応)

- 本番: 「vCPU 2コア / メモリ 1GB」の競技サーバー4台、ベンチ「3コア / 2GB」1台、外部API(isubank・isulogger)「3コア / 2GB」1台
  (公式リポジトリ README「本番当日の動作環境」)
- 初期状態: 4台すべてに同じアプリがデプロイされている(マニュアル)。アプリは systemd から docker-compose で起動
  (README の `isucoin.service`)。ベンチ対象はそのうち1台
- 競技ノード: `official_nodes: 4`、c5.large を `mem_gb: 1`(isucon8-qualify・isucon9-final と同じ扱い)
- 外部API: **ベンチノードに同居させる**(isuenv のベンチノードは1台だけのため。決定)。
  ベンチノードのスペックはベンチと外部APIの合計(6コア / 4GB)に近づける。候補は c7a.2xlarge を 6コア・mem=4G に絞る形だが、
  `ValidCores` を `describe-instance-types` で確認してから決める(catalog.yaml 冒頭の表に追記)
- 分かっている壊れ箇所(要実機確認):
  - `webapp/go/Dockerfile` は `golang:1.11`(Debian stretch)で `apt-get update` するが、stretch は archive.debian.org に移った
  - `mysql:8` がタグ固定されておらず、今は 8.0 系ではない版が来る。当時に近い 8.0 系に固定する
  - `blackbox` の isubank・logger は起動時に `go get ./...` で依存の最新版を取るため、Go 1.11 でビルドできない可能性が高い
  - アプリは起動時に `dep ensure` するので、依存はビルド時に取得済みにしておく
- `isuenv bench`: catalog に `bench` を定義し、ベンチノードから対象ノードのプライベートIPを叩くコマンドを表示する。
  外部APIのURLをベンチにどう渡すかは公式のベンチ実装を読んで決める

### isucon6-final

- 本番: Azure Standard_DS2_v2(2vCPU / 7GiB)の競技サーバー5台(`azure/azuredeploy.json` の vmSize、`regulation.md`「5ノード」)
- 初期状態: **01号機だけアプリが動いていて、2〜5号機は動いていない**(`regulation.md`)。2〜5号機は Docker 入りでアプリ未起動にする。
  これにノード番号の仕組み(isuenv 側の変更 2)を使う
- 競技ノード: `official_nodes: 5`、m5.large を `mem_gb: 7`(isucon6-qualify と同じ再現方法)
- ベンチ: `provisioning/deploy.json` に vmSize が3つ(DS2_v2, DS3_v2, DS2_v2)あり、どれがベンチか要確認
- ベンチの実行: `bench/README.md` の `./local-bench -urls=https://<IP>:443 -timeout 30` 相当
- 5台は c5.large 系で 10 vCPU。README の前提「6 vCPU 以上のクォータ」を超えるので、必要 vCPU を README に書く

### isucon7-final(最後に対応)

- 本番: 競技サーバー4台。初期構成は **先頭3台が Web、残り1台が DB**。各サーバーの `/etc/hosts` に4台の短いホスト名と
  ローカルIPが書かれ、アプリはそのホスト名で DB に接続する(`manual.pdf`「サーバー、ネットワーク構成について」)
- 1台あたりの CPU・メモリとベンチのスペック: マニュアルには見当たらない。公式ブログ等で確認し、無ければ「非公開」
- 本番 OS は Ubuntu 16.04 だが、24.04 で Go 実装のみを動かす(決定)
  - Go 1.9.2 は公式バイナリ。依存は `dep`
  - MySQL は 24.04 の apt では 8.0。README の `GRANT ... IDENTIFIED BY` は 8.0 で通らない。
    当時のドライバーとの認証方式の相性も含め、8.0 で `sql_mode` 等を当時に寄せて動くならそれで、
    動かなければ 5.7 系をどう入れるかを実装者が判断する
  - 参照実装は systemd の `cco.golang.service`(マニュアルの「参照実装の切り替え方法」)。Go を有効にしておく
- `--nodes 4` のとき本番どおり Web 3台 + DB 1台に振り分け、`/etc/hosts` を書く(isuenv 側の変更 2・3)。
  `--nodes 1` のときは1台で完結させる。それ以外の台数の扱いは実装者の判断(README に書く)

## 検証

各問題について、公開前に次を確認する。結果(初期スコア・`up` から SSH 可能までの時間・AMI サイズ)は
catalog.yaml のコメントに「YYYY-MM-DD に実機で確認」として残す(既存の isucon8-qualify と同じ書き方)。

1. `isuenv up <問題> --nodes <本番台数> --bench` で立ち、全ノードに SSH できる
2. 初期状態が本番どおり(動いているサービス・台ごとの役割)
3. `isuenv bench <問題>` が表示するコマンドで、公式スペックの制限下で Go 実装のベンチが pass する
4. `--nodes 1` でも立ち、ベンチが pass する
5. 再起動しても初期状態に戻る(本番のレギュレーションに再起動後の動作要件がある)
6. 既存の問題の `up` が変わらない(テストと、1問で実機確認)

## 進め方

- 8本戦 → 6本戦 → 7本戦の順に、1問ずつ別 PR にする(提案)
- 8本戦の PR に Packer 定義の共通部分・`publish.sh`・`cleanup.sh` を含める
- isuenv 側の変更 2(ノード番号)は6本戦で、変更 3(他ノードのIP)は7本戦で入れる

## 実装者に任せること

- provision スクリプトの中身と、壊れ箇所の具体的な直し方
- ノード情報を渡すファイル名・形式、初期化スクリプトの名前
- 他ノードのIPを決める方式(上記2案から)
- 古いAMIを何世代残すか
- 7本戦で `--nodes` が 1・4 以外のときの役割分担

## 実装メモ(2026-10-24)

実装してみて、設計から変えた・決めたこと。

- **1つの PR にまとめた**(進め方の「1問ずつ別 PR」はやめた)。3問とも同じ日にAMIの作成と検証まで終わったため
- **他ノードのIP**: 案(IP の明示指定 / 2段階起動)ではなく、全ノードの起動後に `isuenv:hosts` タグ(全ノードのプライベートIP)を
  付け、ノードはインスタンスメタデータのタグから読む方式にした。IP の衝突を気にしなくてよく、起動順も変えずに済む。
  タグの反映に数分かかることがあるので、7本戦の初期化は最大15分待つ
- **ノード情報**: user-data が `/etc/isuenv/node.env` に `ISUENV_ROLE` / `ISUENV_NODE_INDEX` / `ISUENV_NODES` / `ISUENV_HAS_BENCH` を書き、
  AMI の `/usr/local/sbin/isuenv-node-init` を実行する
- **常駐プロセスを外す共通処理**(`images/provision/common.sh`): isucon8-final(mem=1G)で、Ubuntu 24.04 の snapd・SSM agent などが
  150MB ほど使っていて2回目以降のベンチでアプリが OOM になったため
- **isucon8-final の外部API**: 当日の証明書(`*.isucon8.flying-chair.net`)は期限切れでドメインも使えないので、外部APIは平文HTTPで受け、
  ベンチは競技ノードの証明書を検証しないようにした。ベンチノードは c7i.2xlarge を 6vCPU / 4GiB に絞った
- **isucon6-final の既定の実装**は Go にした(本番は PHP)
- **isucon7-final の競技ノードのスペック**は公開情報に無かったので制限なし(c5.large のまま)
