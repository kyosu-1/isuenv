# isuenv

ISUCON過去問と[private-isu](https://github.com/catatsuy/private-isu)の練習環境を、
公開AMIからAWS EC2上にコマンド一発で構築・破棄するCLI。

## 対応問題

| 問題名 | 本番の台数 | 競技ノード | ベンチノード | `isuenv bench` | AMI・ベンチ手順 |
| --- | --- | --- | --- | --- | --- |
| `isucon9-qualify` | 3 | `c5.large` | `c5.4xlarge`（12 vCPU に制限） | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon9-qualify) |
| `isucon9-final` | 3 | `c5.large`（メモリ 1GiB に制限） | -（非公開） | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon9-final) |
| `isucon10-qualify` | 3 | `c5.large`（1 vCPU / 2GiB に制限） | `r5.large`（1 vCPU に制限） | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon10-qualify) |
| `isucon10-final` | 3 | 1号機 `c5.large`（メモリ 1GiB に制限）<br>2号機 `c5.large`（メモリ 2GiB に制限）<br>3号機 `c5.xlarge`（メモリ 1GiB に制限） | `c5.2xlarge` | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon10-final) |
| `isucon11-qualify` | 3 | `c5.large` | `c4.xlarge` | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon11-qualify) |
| `isucon11-final` | 3 | `c5.large`（メモリ 2GiB に制限） | `c5.xlarge` | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon11-final) |
| `isucon12-qualify` | 3 | `c5.large` | `c5.xlarge` | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon12-qualify) |
| `isucon12-final` | 5 | `c5.large` | `c5.xlarge` | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon12-final) |
| `isucon13` | 3 | `c5.large` | `c5.2xlarge`（メモリ 8GiB に制限） | - | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon13) |
| `isucon14` | 3 | `c5.large` | `c5.2xlarge`（メモリ 8GiB に制限） | 対応 | [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon/tree/main/isucon14) |
| `private-isu` | 1 | `c7a.large` | `c7a.xlarge` | 対応 | [catatsuy/private-isu](https://github.com/catatsuy/private-isu#ami) |

既定では、競技ノードもベンチノードも**本番の公式スペック**（上流のREADME・当日マニュアル・公式ブログに書かれた値）で立つ。
インスタンスタイプだけでは合わない問題は、CPUとメモリを絞って合わせる（[公式スペックの再現](#公式スペックの再現)）。

- **本番の台数**: 本番の競技サーバーの台数。`isuenv up` の `--nodes` の既定値は 1 のままなので、本番と同じ台数にするには `--nodes` で指定する
- **競技ノード**: `isuenv up` でタイプを指定しなかったときのスペック。「〜に制限」はそのタイプをそこまで絞って起動するという意味（`c5.large`（1 vCPU / 2GiB に制限）なら、OSから見えるのは 1 vCPU・約 2GiB）。本番で競技ノードのスペックが揃っていなかった問題は号機ごとに書いてある
- **ベンチノード**: `--bench` で追加されるベンチマーカー専用ノードのスペック。`-` は本番のベンチのスペックが公開されていない問題で、`--bench-instance-type` での明示指定が要る
- **`isuenv bench`**: ベンチ実行コマンドをIPを埋めた状態で表示できるか。`-` の問題はリンク先の手順を読んで手で打つ

手元のバージョンでの最新の一覧は `isuenv problems` で確認できる。

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
isuenv problems               # 対応問題一覧
isuenv up isucon13            # 環境作成（1台, TTL 8h, c5.large）
isuenv up private-isu         # private-isu（1台, TTL 8h, c7a.large）
isuenv up isucon13 --nodes 3  # 本番同様の3台構成
isuenv up private-isu --bench # 競技1台 + ベンチマーカー専用1台（別のインスタンスタイプ）
isuenv list                   # 稼働中環境と概算コスト・残りTTL
isuenv ssh isucon13           # 1号機にSSH（isucon13-2 で2号機）
isuenv bench isucon14         # ベンチ実行コマンドを表示（IPを埋めた状態で）
isuenv down isucon13          # 環境削除
isuenv nuke                   # isuenv管理の全リソース削除（VPC・キーペア含む）
isuenv version                # バージョン表示
```

## コマンドリファレンス

### `isuenv up <問題名> [flags]`

環境を作成し、全ノードがrunningかつパブリックIPが付くまで待ってから結果を表示する。

起動に使うAMIは問題ごとの名前パターンで**毎回いちばん新しいものが選ばれる**（AMI IDは固定していない）。
上流が同じパターンのままAMIを差し替えることがあるので、解決したAMIのIDと名前を起動前に表示する。

```
$ isuenv up isucon14
Resolving AMI for isucon14...
  -> ami-0fcf9e8e8675a9ee4 (isucon14-20260818100152)
Ensuring network...
```

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `--ttl` | `8h` | この時間が経過したら自動でterminateする（[挙動](#ttlの挙動)） |
| `--nodes` | `1` | 起動台数。1以上 |
| `--instance-type` | 問題ごと | 全競技ノードのEC2インスタンスタイプ。既定値は問題ごとに異なり、`isuenv problems` の TYPE 列で確認できる（ほとんどは `c5.large`、private-isuは推奨に合わせて `c7a.large`）。指定するとその問題のCPU制限は外れる（[タイプを指定したとき](#タイプを指定したとき)） |
| `--node-instance-types` | 問題ごと | 競技ノードごとのインスタンスタイプを、1号機から順にカンマ区切りで指定する（[ノードごとのスペック](#ノードごとのスペック)）。`--instance-type` とは併用できない |
| `--bench` | `false` | ベンチマーカー専用ノードを1台追加する。スペックは `isuenv problems` の BENCH TYPE 列の値（上流の推奨タイプ、または本番のベンチのスペック）。本番のベンチのスペックが非公開の問題（isucon9-final）ではエラーになる |
| `--bench-instance-type` | なし | ベンチマーカー専用ノードのインスタンスタイプを明示する。指定すると `--bench` は省略できる。その問題のベンチのCPU制限は外れる |
| `--no-limits` | `false` | 公式スペックに合わせるためのCPU・メモリの制限を全て外し、インスタンスタイプそのままで起動する |

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

#### 公式スペックの再現

本番の競技サーバーやベンチマーカーと同じ形のインスタンスタイプがAWSに無い問題は、
近いタイプを次の2つの方法で絞って合わせる。どの問題で何を絞るかは[対応問題](#対応問題)の表と
`isuenv problems` で確認できる。

| 絞るもの | 方法 | 例 |
| --- | --- | --- |
| CPU | 起動時（RunInstances）の `CpuOptions` でコア数とコアあたりのスレッド数を指定する | isucon10-qualify の競技ノード: `c5.large` を 1コア×1スレッド = 1 vCPU に<br>isucon9-qualify のベンチ: `c5.4xlarge` を 6コア×2スレッド = 12 vCPU に |
| メモリ | カーネル引数 `mem=<N>G`。上流の provisioning が本番で使っていたのと同じ方法 | isucon11-final の競技ノード: `mem=2G`<br>isucon13 / isucon14 のベンチ: `mem=8G` |

メモリを絞るノードは、**起動直後に1回だけ自動で再起動する**（カーネル引数は起動時にしか効かないため。
user-dataが `/etc/default/grub.d/zz-isuenv-mem.cfg` を書いて `update-grub` し、再起動する）。
`up` が結果を表示した直後の1〜2分は、sshがつながらなかったり途中で切れたりすることがある。
IPは変わらないので、少し待ってつなぎ直せばよい。

```
$ isuenv up isucon10-qualify --nodes 3 --bench
...
  isucon10-qualify-1      public 1.2.3.4  private 10.100.0.1  c5.large(1vCPU,mem=2G)  app
  isucon10-qualify-2      public 1.2.3.5  private 10.100.0.2  c5.large(1vCPU,mem=2G)  app
  isucon10-qualify-3      public 1.2.3.6  private 10.100.0.3  c5.large(1vCPU,mem=2G)  app
  isucon10-qualify-bench  public 1.2.3.7  private 10.100.0.4  r5.large(1vCPU)         bench

Nodes with a mem= limit reboot once right after launch to apply it; ssh may be refused for a minute or two.
```

制限が効いているかはノードの中で確認できる。

```sh
nproc              # CpuOptions で絞った後のvCPU数
cat /proc/cmdline  # mem=2G が入っていれば適用済み
free -m            # mem=2G なら total は約1.9GB（カーネルが予約する分だけ少なく見える）
```

知っておくこと:

- **料金は変わらない。** 課金はインスタンスタイプで決まるので、絞っても安くはならない
- AMIが既に同じ `mem=` で起動する場合は何もせず、再起動もしない。別の値の `mem=` が入っている場合は、それを取り除いてから足す（二重指定にしない）
- 搭載メモリが制限値以下のタイプを指定した場合（例: `mem=8G` の問題に 8GiB の `c5.xlarge`）も何もせず、再起動もしない。表示上の `mem=` は残る
- 再起動は1回だけ。何かの理由で制限が効かなくても、再起動を繰り返すことはない（その場合は制限なしで動き続ける）
- TTLによる自動terminateは、この再起動の前に仕込まれる。再起動しても期限は維持される（[TTLの挙動](#ttlの挙動)）
- isucon10 の「1コア」「2コア」が物理コアを指すのか論理スレッドを指すのかは、公開情報からは断定できない。isuenvはvCPU数（スレッド数）として再現する
- 本番のスペックが公開されていないもの（isucon9-final のベンチ、isucon9-qualify のベンチのメモリ）は、推測で埋めずに未設定のままにしている

#### タイプを指定したとき

インスタンスタイプを自分で指定したノードは、次のように扱う。

| 指定 | タイプ | CPU制限 | メモリ制限 |
| --- | --- | --- | --- |
| なし（既定） | 問題ごとの値 | 適用 | 適用 |
| `--instance-type` / `--node-instance-types` / `--bench-instance-type` | 指定したタイプ | **外す** | 適用 |
| `--no-limits` | 問題ごとの値（タイプの指定があればそれ） | 外す | 外す |

- CPU制限を外すのは、`CpuOptions` に指定できる値がタイプごとに違い、問題ごとの値が指定されたタイプで有効とは限らないため。
  問題の既定と同じタイプを指定した場合も外れる
- メモリ制限（`mem=`）はタイプに依らず効くので、タイプを変えても残す。問題が前提にしているメモリ量のまま、CPUだけ変えて試せる
- 制限をどちらも外して素のインスタンスタイプで立てたいときは `--no-limits` を付ける。競技ノードとベンチノードの両方に効く

```sh
isuenv up isucon10-qualify                            # c5.large を 1 vCPU / 2GiB に制限（本番相当）
isuenv up isucon10-qualify --instance-type c6i.large  # c6i.large の 2 vCPU のまま、メモリだけ 2GiB に制限
isuenv up isucon10-qualify --no-limits                # 素の c5.large（2 vCPU / 4GiB）
isuenv up isucon13 --bench --no-limits                # ベンチも素の c5.2xlarge（8 vCPU / 16GiB）
```

#### ノードごとのスペック

本番で競技ノードのスペックが揃っていなかった問題は、ノードごとのスペックを持っている
（`isuenv problems` の TYPE 列に1号機から順に並ぶ）。いまは isucon10-final だけで、本番と同じく
1号機が2コア / 1GiB、2号機が2コア / 2GiB、3号機が4コア / 1GiB になる。

```sh
isuenv up isucon10-final --nodes 3 --bench   # c5.large(mem=1G), c5.large(mem=2G), c5.xlarge(mem=1G) + ベンチ c5.2xlarge（本番相当）
isuenv up isucon10-final --nodes 3 --instance-type c5.large               # 全ノード c5.large（メモリ制限は号機ごとのまま）
isuenv up isucon13 --nodes 3 --node-instance-types c5.xlarge,c5.large     # 1号機だけ c5.xlarge
```

`--nodes` が本番の台数と違うとき:

- **少ないとき**は、1号機から順にその台数ぶんだけ使う（`--nodes 1` なら1号機のスペックの1台）
- **多いとき**は、本番に無い号機（isucon10-final なら4号機以降）が問題の既定スペックになる。isucon10-final の既定スペックは1号機と同じ `c5.large(mem=1G)`

タイプの指定との関係:

- `--instance-type` は全ノードを同じタイプにする指定で、ノードごとのタイプより優先する
- `--node-instance-types` で指定が足りない番号のノードは、その号機の問題ごとのスペックのまま
- 競技ノードのスペックが揃っていない構成では、`up` の結果にタイプとロールが並ぶ

isucon10-final のAMIは、`contestant.slice` が全ノードで envoy・MySQL・アプリの合計を 1200M に制限している。
2号機は `mem=2G` でOS全体が 2GiB になるが、この slice の上限は 1200M のまま。

#### ベンチマーカー専用ノード

ISUCONのベンチマーカーには、競技ノードとは別にスペックが決まっている（本番でも競技サーバーとは別の
マシンで動いていた）。`--bench` は、そのスペックのベンチ用ノードを競技ノードとは別に1台追加する。

スペックは問題ごとに決まっていて、`isuenv problems` の BENCH TYPE 列で確認できる。競技ノードと同じく、
タイプだけで合わない問題はCPUやメモリを絞る（[公式スペックの再現](#公式スペックの再現)）。
別のタイプにしたいときは `--bench-instance-type` で指定する（[タイプを指定したとき](#タイプを指定したとき)）。

```sh
isuenv up private-isu --bench                                     # 競技1台(c7a.large) + ベンチ1台(c7a.xlarge)
isuenv up private-isu --nodes 3 --bench-instance-type c7a.2xlarge # 競技3台 + ベンチ1台(c7a.2xlarge)
isuenv up private-isu --nodes 4                                   # 従来どおりベンチノードなし
```

ベンチノードの名前は番号ではなく `<問題名>-bench` になる（EC2のNameタグもsshのホスト名も同じ）。
`isuenv ssh private-isu-bench` で入れる。番号付きの名前（`private-isu-1` など）は競技ノードだけを指す。
ベンチノードがある構成では、`up` の結果にタイプ（制限があればそれも）とロールが並ぶ。

```
$ isuenv up private-isu --bench
...
  private-isu-1      public 1.2.3.4  private 10.100.0.1  c7a.large   app
  private-isu-bench  public 5.6.7.8  private 10.100.0.2  c7a.xlarge  bench
```

### `isuenv list`

稼働中の環境を一覧する。

| 列 | 内容 |
| --- | --- |
| `ENV` | 問題名 |
| `NODES` | 台数 |
| `TYPE` | インスタンスタイプ。ベンチノードがある場合は `c7a.large +bench c7a.xlarge` のように混在を表す。CPUやメモリを絞って起動したノードは `c5.large(1vCPU,mem=2G)` のように制限を添える |
| `UPTIME` | 起動からの経過時間 |
| `EST COST` | 概算費用。ノードごとの単価で合算する。あくまで目安 |
| `TTL LEFT` | 自動terminateまでの残り時間 |
| `PUBLIC IPS` | 各ノードのパブリックIP |

### `isuenv ssh <問題名>[-N|-bench]`

ノードにSSHする。番号を省略すると1号機に繋ぐ（`isucon13` = `isucon13-1`）。
ベンチマーカー専用ノードには `isucon13-bench` で繋ぐ。

実行のたびに次の2つを行うので、**グローバルIPが変わったら打ち直せば復旧する**。

- セキュリティグループのingressを、現在のグローバルIPで貼り直す
- `~/.ssh/isuenv_config` を稼働中の環境から再生成する

生成されたssh configは `~/.ssh/config` からIncludeされるので、素の `ssh isucon13-1` やVS Code Remoteからも使える。

### `isuenv bench <問題名>`

稼働中の環境に対してベンチマーカーを実行するコマンドを**表示する**。実行はしない。

```
$ isuenv bench isucon14
ssh isucon14-bench 'cd /home/isucon && sudo -u isucon ./bench run --addr 10.100.0.5:443 --target https://isuride.xiv.isucon.net --payment-url http://10.100.0.9:12346 --payment-bind-port 12346'
```

ベンチマーカーはAMIに同梱されているが、**起動方法が問題ごとに全く違い、しかも引数に埋めるprivate IPが構成によって変わる**。
そこを毎回上流のREADMEを見ながら手で埋めるのが練習環境での一番の手間なので、そこだけを引き受ける。

- ベンチマーカー専用ノード（`--bench`）があればそのノードで打つコマンドを出す。無ければ1号機
- ベンチ対象は1号機。`{{.TargetIP}}` にそのprivate IPが入る
- `{{.BenchIP}}` にはベンチを打つノード自身のprivate IPが入る（isucon14のpaymentサーバのように、ベンチ側がbindするアドレスを引数で渡す問題で使う）

**実行しないのは意図的**。ISUCONではベンチの前後（デプロイ、ログ退避、集計）を自前のMakefileやスクリプトで回すのが定番で、
実行まで奪うとそこに組み込みづらくなる。出力するだけなら好きに合成できる。

```sh
$(isuenv bench isucon14)                    # そのまま実行
isuenv bench isucon14 >> Makefile           # Makefileに取り込む
```

対応している問題は `isuenv problems` の BENCH CMD 列が `yes` のものだけ。
起動方法は実機で確認しないと確定できないため、検証できた問題から順に埋めている（現在は isucon14 と private-isu）。
未対応の問題では、NOTESのリンク先を見るよう促すエラーになる。

### `isuenv down <問題名>`

その環境のインスタンスをterminateする。VPC・サブネット・SG・キーペアは残るので、次の `up` で再利用される。対象が無い場合も成功扱い。

### `isuenv nuke`

isuenv管理下の**全リソース**を削除する。`yes` の入力を求められる。インスタンスの終了を待ってから、キーペア・SG・サブネット・IGW・VPCの順に消す。

### `isuenv problems`

対応している問題と、SSHユーザー、本番の競技サーバーの台数（NODES）、競技ノードのスペック（TYPE。ノードごとのスペックがある問題は1号機から順）、
ベンチマーカー専用ノードのスペック（BENCH TYPE。本番のスペックが非公開の問題は `-`）、`isuenv bench` が使えるか（BENCH CMD）、ベンチ手順へのリンクを一覧する。

TYPE と BENCH TYPE は、タイプを絞って起動する問題では制限を括弧で添える。

| 表示 | 意味 |
| --- | --- |
| `c5.large` | `c5.large` をそのまま起動する |
| `c5.large(mem=2G)` | `c5.large` をメモリ 2GiB に制限する |
| `c5.4xlarge(12vCPU)` | `c5.4xlarge` を 12 vCPU に制限する |
| `c5.large(1vCPU,mem=2G)` | `c5.large` を 1 vCPU / 2GiB に制限する |

## TTLの挙動

TTLの実体はインスタンス内の仕組みで、次の3段構えで動く。

1. 起動時のuser-dataが絶対期限（UNIX時刻）を `/var/lib/isuenv-expires-at` に書く
2. `/etc/cron.d/isuenv-ttl` が毎分その時刻を過ぎたか判定し、過ぎていれば `shutdown -P now`
3. インスタンスは `instance-initiated-shutdown-behavior=terminate` で起動しているため、停止ではなく**terminate**される（EBSごと消えるので課金が完全に止まる）

絶対時刻をディスクに持つので、**リブートしても期限は維持される**（メモリ制限を適用するための起動直後の再起動も含む）。判定が毎分なので、実際にterminateされるのは期限から1分程度あと。**ノートPCを閉じてもCLIを終了しても効く。**

## 作成されるリソース

AWS上のリソースはすべて `isuenv:managed=true` タグが付き、`nuke` の対象はこのタグで判定される。

| リソース | 内容 |
| --- | --- |
| VPC | `10.100.0.0/16` |
| サブネット | `10.100.0.0/24`（パブリックIP自動割当ON） |
| インターネットゲートウェイ | VPCにアタッチし、メインルートテーブルに `0.0.0.0/0` を向ける |
| セキュリティグループ | 名前 `isuenv`。**実行時のグローバルIP/32からのtcp 22/80/443** と、**自身のSGからの全プロトコル**（ノード間通信用） |
| キーペア | 名前 `isuenv` |
| EC2インスタンス | `--nodes` の台数（`--bench` 指定時はベンチ用に+1台） |

インスタンスには `isuenv:env`（問題名）、`isuenv:node`（何号機か）、`isuenv:role`（`app` = 競技ノード /
`bench` = ベンチマーカー専用ノード）、`isuenv:expires-at`（TTLの絶対期限）のタグが付く。
CPUやメモリを絞って起動したノードには、`isuenv:vcpus`（絞った後のvCPU数）と `isuenv:mem-gb`（`mem=` のGiB数）も付く。
CLIはローカルに状態を持たず、すべてこれらのタグから復元する。

ローカルには次のファイルが作られる。

| パス | 内容 |
| --- | --- |
| `~/.ssh/isuenv.pem` | キーペアの秘密鍵（`0600`）。AWS側にキーペアが無いときに作成され、**このファイルも上書きされる**（`nuke` 後の `up` など） |
| `~/.ssh/isuenv_config` | ホスト定義。`up` / `ssh` / `down` のたびに再生成される |
| `~/.ssh/config` | 先頭に `Include` 行を一度だけ追加（パスは絶対パスで書かれる） |

VPC・サブネット・IGW・SG・キーペアは**無料**なので、`down` 後にこれらが残っていても費用は発生しない。

## コストの目安

ap-northeast-1のオンデマンド概算で、c5.large（多くの問題の既定）が約$0.107/時、
c7a.large（private-isuの既定）が約$0.129/時、c7a.xlarge（private-isuのベンチ用）が約$0.258/時。
ベンチノードで大きいのは c5.2xlarge（isucon10-final / isucon13 / isucon14）の約$0.428/時と、
c5.4xlarge（isucon9-qualify）の約$0.856/時。CPUやメモリを絞っても単価は元のタイプのまま変わらない。
`isuenv list` の EST COST はノードごとの単価を合算した概算であり、実際の課金はAWSの請求を確認すること。

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

### 公式スペックの制限

**CPU・メモリの制限は実機でしか検証できない。** カタログの `cpu_options` / `mem_gb` や、user-dataのメモリ制限の処理を変えたときは以下を実施する。
AMIによってgrubの構成が違いうるので、`mem_gb` を新しく設定した問題はその問題のAMIで確認すること。

1. `./isuenv up isucon10-qualify --bench --ttl 1h` — 起動時の表示が `c5.large(1vCPU,mem=2G)` と `r5.large(1vCPU)` で、再起動の案内が出ること
2. 1〜2分待ってから `./isuenv ssh isucon10-qualify` — 再起動後にログインできること（IPが変わっていないこと）
3. ノードの中で確認する:
   ```sh
   nproc                               # 1
   cat /proc/cmdline                   # mem=2G が1つだけ入っていること
   free -m                             # total が約1.9GB
   uptime                              # 再起動が1回だけであること（起動から数分たっても再起動を繰り返していない）
   cat /var/lib/isuenv-mem-applied     # 2G
   cat /var/lib/isuenv-expires-at      # TTLの期限が残っていること
   cat /etc/cron.d/isuenv-ttl          # TTLのcronが残っていること
   sudo tail /var/log/cloud-init-output.log   # isuenv: のメッセージ
   ```
4. `./isuenv ssh isucon10-qualify-bench` — `nproc` が 1、`free -m` が約16GB（ベンチはメモリを絞らないので再起動もしない）
5. `./isuenv list` — TYPE が `c5.large(1vCPU,mem=2G) +bench r5.large(1vCPU)` であること
6. アプリが起動していること（ブラウザで `http://<public ip>`）。メモリを絞った状態でサービスが立ち上がるかの確認
7. `./isuenv down isucon10-qualify`
8. `./isuenv up isucon10-qualify --no-limits --ttl 1h` — `nproc` が 2、`/proc/cmdline` に `mem=` が無く、再起動しないこと → `down`

### `isuenv bench`

**カタログのベンチ起動方法は実機でしか検証できない。** 問題を足したときは必ず以下を実施する。

1. `./isuenv up isucon14 --nodes 3 --bench-instance-type c5.xlarge --ttl 1h`
2. `./isuenv bench isucon14` — ベンチノード（`isucon14-bench`）を指し、`--addr` が1号機のprivate IP、`--payment-url` がベンチノードのprivate IPになっていること
3. **出力されたコマンドをそのまま実行し、ベンチが完走してスコアが出ること。** ここが本番
4. `./isuenv bench isucon13` — 未対応の問題として、NOTESを見るよう促すエラーになること
5. `./isuenv down isucon14`

ベンチノードなしの構成も確認する。

1. `./isuenv up private-isu --ttl 1h`（1台完結）
2. `./isuenv bench private-isu` — 1号機を指し、`-t http://<1号機のprivate IP>` になっていること
3. 出力されたコマンドを実行し、完走すること
4. `./isuenv up private-isu --bench` の構成では `private-isu-bench` を指し、`-t` が1号機を向くこと

### private-isu

private-isuは提供元AMIがmatsuu/aws-isuconと別物なので、TTL（user-data）が効くかを個別に確認する。

1. `./isuenv up private-isu --ttl 15m` — `c7a.large` で起動すること
2. `./isuenv list` — EST COST が `-` でなく金額で出ること（`cost.go` に c7a.large の単価があること）
3. `./isuenv ssh private-isu` — ログインでき、ブラウザで `http://<public ip>` が見えること
4. **TTLの実体確認**（ここが効かないと課金が止まらない）:
   ```sh
   cat /var/lib/isuenv-expires-at   # UNIX時刻が入っていること
   ls -l /etc/cron.d/isuenv-ttl     # 存在すること
   ```
5. ベンチが通ること:
   ```sh
   sudo su - isucon
   /home/isucon/private_isu/benchmarker/bin/benchmarker \
     -u /home/isucon/private_isu/benchmarker/userdata -t http://localhost
   ```
6. 15分後に実際にterminateされること（`./isuenv list` が空になる）
7. ベンチ専用ノード構成: `./isuenv up private-isu --bench --ttl 1h` → `./isuenv list` の TYPE が
   `c7a.large +bench c7a.xlarge`、EST COST が2台の単価の合算になること →
   `./isuenv ssh private-isu-bench` でベンチ機（`c7a.xlarge`）に入れること → `./isuenv down private-isu`

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

- ベンチマーカーはAMIに同梱されている。実行方法は問題ごとに異なるので、`isuenv bench <問題名>` でコマンドを出すか、`isuenv problems` のNOTESのリンク先を参照
- 消し忘れてもTTLで自己消滅するが、`isuenv list` での確認を習慣にすること

## ライセンス

MIT License. 詳細は [LICENSE](LICENSE) を参照。

利用しているAMIは [matsuu/aws-isucon](https://github.com/matsuu/aws-isucon)（MIT）と
[catatsuy/private-isu](https://github.com/catatsuy/private-isu)（MIT）が公開しているもので、
本ツールはそれらのAMIを起動・破棄するだけであり、AMIそのものは配布していない。
