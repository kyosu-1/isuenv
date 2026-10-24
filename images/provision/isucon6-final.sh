#!/bin/bash
# isucon6-final(ISUCON6 本戦)のAMIを作る。本番と同じく docker-compose でアプリを動かす
# (上流README「DockerだけインストールされていればOS等は問わない」)。
#
# 1つのAMIで競技ノードとベンチノードの両方を兼ねる。どちらとして動くかは起動時に isuenv の user-data が
# /etc/isuenv/node.env に書き、/usr/local/sbin/isuenv-node-init がそれを見てサービスを切り替える。
#
#   /home/isucon/webapp  本番と同じ配置(上流 ansible の app ロール)。isu.service で起動する
#   /home/isucon/bench   ベンチマーカー(local-bench)
#
# 本番との違い: 本番の初期状態は PHP 実装(docker-compose.yml -> docker-compose-php.yml)だったが、
# isuenv では Go 実装にしてある。他の言語の Dockerfile は当時のままで、今ビルドできるかは確認していない。
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive

# 2026-10-24 時点の isucon/isucon6-final の master
REPO_URL=https://github.com/isucon/isucon6-final
REPO_REV=6b7e659f3de2f455b1f76bf27531534a20f0f79f
# 本戦(2016-12-03)当時の mysql:latest。上流の compose はタグ無しの mysql で、今は 9 系が来る
MYSQL_IMAGE=mysql:5.7.16

HOME_DIR=/home/isucon
SRC=/tmp/isucon6-final

sudo apt-get update
sudo apt-get install -y --no-install-recommends docker.io docker-compose-v2 git make curl ca-certificates go-bindata

sudo useradd -m -s /bin/bash -G docker isucon
echo 'isucon ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/isucon
sudo chmod 755 "$HOME_DIR"

# 本番の isu.service は /usr/local/bin/docker-compose(v1.11.2)を使っていた。compose v2 に渡すだけのラッパーを置く。
sudo tee /usr/local/bin/docker-compose >/dev/null <<'EOF'
#!/bin/sh
exec docker compose "$@"
EOF
sudo chmod 755 /usr/local/bin/docker-compose

git clone "$REPO_URL" "$SRC"
git -C "$SRC" checkout -q "$REPO_REV"

# ---- 競技用アプリ ----------------------------------------------------------------------------
APP="$HOME_DIR/webapp"
sudo cp -a "$SRC/webapp" "$APP"

# Go 実装の依存 goji.io は独自ドメインのインポートパスで、今は goji.io から取得元(VCS)を辿れず glide install が失敗する。
# 実体の GitHub リポジトリを glide に直接教える。
for f in glide.yaml glide.lock; do
  sudo sed -i -e "/^- \(package\|name\): goji.io\$/a\\  repo: https://github.com/goji/goji\n  vcs: git" "$APP/go/$f"
done
grep -q 'repo: https://github.com/goji/goji' "$APP/go/glide.lock"
sudo sed -i -e "s@image: mysql\$@image: $MYSQL_IMAGE@" "$APP/docker-compose-mysql.yml"
grep -q "image: $MYSQL_IMAGE" "$APP/docker-compose-mysql.yml"
sudo ln -sfn docker-compose-go.yml "$APP/docker-compose.yml"
sudo chown -R isucon:isucon "$HOME_DIR"

# 本番の /etc/systemd/system/isu.service(上流 ansible/playbook/roles/app/files)そのまま
sudo tee /etc/systemd/system/isu.service >/dev/null <<'EOF'
[Unit]
Description=isu-service container
Requires=docker.service
After=docker.service

[Service]
Restart=on-failure
User=isucon
Group=isucon
ExecStart=/usr/local/bin/docker-compose -f /home/isucon/webapp/docker-compose.yml up
ExecStop=/usr/local/bin/docker-compose -f /home/isucon/webapp/docker-compose.yml stop

[Install]
WantedBy=multi-user.target
EOF

sudo -u isucon -H docker compose -f "$APP/docker-compose.yml" build

# 一度起動して、初期データの投入(webapp/sql の 02_initial_data.sql.gz、webapp/.data に入る)をビルド時に済ませる。
# 投入中もアプリは応答するので、mysql コンテナの初期化完了のログを待つ。途中で止めるとデータが欠けたまま
# 「初期化済み」になり、二度と投入されない。
sudo systemctl daemon-reload
sudo systemctl start isu.service
for i in $(seq 1 120); do
  if sudo docker logs webapp-mysql-1 2>&1 | grep -q 'MySQL init process done' \
    && curl -sk -o /dev/null -w '%{http_code}' https://localhost/ | grep -q '^200$'; then
    echo "webapp is up"
    break
  fi
  if [ "$i" -eq 120 ]; then
    sudo journalctl -u isu.service --no-pager | tail -50
    exit 1
  fi
  sleep 10
done
sudo systemctl stop isu.service
sudo systemctl enable isu.service

# ---- ベンチマーカー --------------------------------------------------------------------------
# 本番のベンチは当時の Go でビルドされている(net/http を同梱して手を入れている)ので、当時の golang:1.8 の中でビルドする。
# 初期データを埋め込む seed/bindata.go は go-bindata で作る(上流の jteeuwen/go-bindata は GitHub から消えたので、
# Ubuntu の go-bindata パッケージを使う。生成されるコードは標準ライブラリだけを使う)。
(cd "$SRC/bench/seed" && go-bindata -pkg seed data/*.json)
BENCH="$HOME_DIR/bench"
sudo mkdir -p "$BENCH"
sudo docker run --rm -v "$SRC:/src:ro" -v "$BENCH:/out" golang:1.8 sh -c '
  set -e
  go get github.com/Masterminds/glide
  R=$GOPATH/src/github.com/isucon/isucon6-final
  mkdir -p $R
  cp -a /src/bench /src/portal $R/
  cd $R/bench
  glide install
  go build -o /out/local-bench ./cmd/bench
'
sudo docker rmi golang:1.8
sudo chown -R isucon:isucon "$BENCH"
test -x "$BENCH/local-bench"

# ---- 起動時のノードごとの切り替え ---------------------------------------------------------------
# isuenv の user-data が /etc/isuenv/node.env(ISUENV_ROLE=app|bench, ISUENV_NODE_INDEX, ISUENV_NODES, ISUENV_HAS_BENCH)を
# 書いてからこれを実行する。何度実行しても同じ結果になるようにする(メモリ制限の再起動後に再実行されうる)。
sudo tee /usr/local/sbin/isuenv-node-init >/dev/null <<'EOF'
#!/bin/sh
# isucon6-final: 本番の初期状態は「01 のみでアプリケーションが動いている」(上流 regulation.md)。
# 1号機だけアプリを動かし、2号機以降とベンチノードでは止めておく(Docker とイメージは入っている)。
set -e
. /etc/isuenv/node.env
if [ "$ISUENV_ROLE" = app ] && [ "$ISUENV_NODE_INDEX" = 1 ]; then
  systemctl enable --now isu.service
else
  systemctl disable --now isu.service
fi
EOF
sudo chmod 755 /usr/local/sbin/isuenv-node-init

# ---- 後始末 -------------------------------------------------------------------------------------
sudo apt-get purge -y --auto-remove go-bindata
sudo rm -rf "$SRC"
sudo apt-get clean
sudo sh -c 'rm -rf /var/lib/apt/lists/*'
