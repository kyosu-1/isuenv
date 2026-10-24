#!/bin/bash
# isucon8-final(ISUCON8 本戦)のAMIを作る。Ubuntu 24.04 の上で、本番と同じく docker-compose でアプリを動かす。
#
# 1つのAMIで競技ノードとベンチノードの両方を兼ねる。どちらとして動くかは起動時に isuenv の user-data が
# /etc/isuenv/node.env に書き、/usr/local/sbin/isuenv-node-init がそれを見てサービスを切り替える。
#
#   /home/isucon/isucon2018-final  本番当日と同じ配置(docs と webapp)。isucoin.service で起動する
#   /home/isucon/bench             ベンチマーカー(bin/bench)
#   /home/isucon/blackbox          外部API(isubank・isulogger)。本番は別VMだったが、ベンチノードに同居させる
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive

# 2026-10-24 時点の isucon/isucon8-final の master
REPO_URL=https://github.com/isucon/isucon8-final
REPO_REV=38c4f6e20388d1c4f1ed393fb75b38d472e44abf
# 当日(2018-10-20)の mysql:8 タグが指していた版。上流の compose は mysql:8 のままで、今は 8.0 ではない版が来る
MYSQL_IMAGE=mysql:8.0.12
# ベンチと外部APIのビルドに使う Go。当日の webapp と同じ 1.11 系
GO_VERSION=1.11.13

HOME_DIR=/home/isucon
SRC=/tmp/isucon8-final

sudo apt-get update
sudo apt-get install -y --no-install-recommends docker.io docker-compose-v2 git make curl ca-certificates

sudo useradd -m -s /bin/bash -G docker isucon
echo 'isucon ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/isucon
sudo chmod 755 "$HOME_DIR"

# 当日の isucoin.service は /usr/local/bin/docker-compose を使っていた。compose v2 に渡すだけのラッパーを置き、
# README・マニュアルにある docker-compose のコマンドがそのまま打てるようにする。
sudo tee /usr/local/bin/docker-compose >/dev/null <<'EOF'
#!/bin/sh
exec docker compose "$@"
EOF
sudo chmod 755 /usr/local/bin/docker-compose

git clone "$REPO_URL" "$SRC"
git -C "$SRC" checkout -q "$REPO_REV"

# ---- 競技用アプリ ----------------------------------------------------------------------------
APP="$HOME_DIR/isucon2018-final"
sudo mkdir -p "$APP"
sudo cp -a "$SRC/docs" "$SRC/webapp" "$APP/"

# golang:1.11 は今は buster 版を指し、buster も stretch も apt の取得元が archive.debian.org に移っていて
# apt-get update が 404 で失敗する。当日の golang:1.11 は stretch だったので、stretch に固定して取得元を差し替える。
# (buster には mysql-client パッケージが無いので、stretch にしないと次の apt-get install も通らない)
sudo tee "$APP/webapp/go/Dockerfile" >/dev/null <<'EOF'
FROM golang:1.11-stretch

# isuenv: stretch の apt は archive.debian.org に移ったので取得元を差し替える
RUN sed -i -e "s,deb.debian.org,archive.debian.org,g" -e "s,security.debian.org,archive.debian.org,g" -e "/stretch-updates/d" /etc/apt/sources.list && apt-get update && apt-get -y install mysql-client
RUN curl -sL https://github.com/golang/dep/releases/download/v0.5.0/dep-linux-amd64 > /usr/bin/dep && chmod +x /usr/bin/dep
EOF
sudo sed -i -e "s@image: mysql:8\$@image: $MYSQL_IMAGE@" "$APP/webapp/docker-compose.yml"
grep -q "image: $MYSQL_IMAGE" "$APP/webapp/docker-compose.yml"

sudo chown -R isucon:isucon "$HOME_DIR"

# 当日の /etc/systemd/system/isucoin.service(上流README)そのまま
sudo tee /etc/systemd/system/isucoin.service >/dev/null <<'EOF'
[Unit]
Description = isucoin application

[Service]
LimitNOFILE=102400
LimitNPROC=102400

WorkingDirectory=/home/isucon/isucon2018-final/webapp

ExecStartPre = /usr/local/bin/docker-compose -f docker-compose.yml -f docker-compose.go.yml build
ExecStart = /usr/local/bin/docker-compose -f docker-compose.yml -f docker-compose.go.yml up
ExecStop = /usr/local/bin/docker-compose -f docker-compose.yml -f docker-compose.go.yml down

Restart   = always
Type      = simple
User      = isucon
Group     = isucon

[Install]
WantedBy = multi-user.target
EOF

# 他言語の実装に切り替えたときに pull を待たなくて済むよう、使うイメージは全部取っておく
sudo docker pull -q "$MYSQL_IMAGE"
sudo docker pull -q nginx:1.15.3-alpine

# mysql コンテナの初期データ投入(docker-entrypoint-initdb.d)が最後まで終わったかを見る。
# 投入中もソケット経由のクエリやアプリの静的ファイルは応答するので、それでは判定できない。途中で止めると
# データが欠けたまま「初期化済み」になり、二度と投入されない。
mysql_initialized() {
  sudo docker logs "$1" 2>&1 | grep -q 'MySQL init process done'
}

# 一度起動して、初期データの投入(z_initializedata.sql.gz)と dep ensure をビルド時に済ませる。
# アプリは起動のたびに dep ensure してから go run するので、vendor が揃っていれば起動が速い。
sudo systemctl daemon-reload
sudo systemctl start isucoin.service
for i in $(seq 1 120); do
  if mysql_initialized webapp-mysql-1 && curl -sk -o /dev/null -w '%{http_code}' https://localhost/ | grep -q '^200$'; then
    echo "isucoin is up"
    break
  fi
  if [ "$i" -eq 120 ]; then
    sudo journalctl -u isucoin.service --no-pager | tail -50
    exit 1
  fi
  sleep 10
done
sudo systemctl stop isucoin.service
sudo systemctl enable isucoin.service

# ---- Go(ベンチと外部API用) ------------------------------------------------------------------
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | sudo tar -C /usr/local -xz
sudo mv /usr/local/go /usr/local/go1.11
GO=/usr/local/go1.11/bin/go

# ---- ベンチマーカー --------------------------------------------------------------------------
BENCH="$HOME_DIR/bench"
sudo cp -a "$SRC/bench" "$BENCH"
# 当日の証明書(*.isucon8.flying-chair.net)は期限切れで、isuenv ではIPアドレスで叩くので名前も合わない。
# ベンチから競技ノードへのHTTPSだけ証明書の検証を外す。
sudo sed -i -e 's@transport := &http.Transport{}@transport := \&http.Transport{TLSClientConfig: \&tls.Config{InsecureSkipVerify: true}} // isuenv: 当日の証明書は期限切れでIPとも合わない@' "$BENCH/src/bench/client.go"
sudo sed -i -e '0,/^import (/s@^import (@import (\n\t"crypto/tls"@' "$BENCH/src/bench/client.go"
grep -q InsecureSkipVerify "$BENCH/src/bench/client.go"
sudo chown -R isucon:isucon "$BENCH"
sudo -u isucon -H env PATH="/usr/local/go1.11/bin:$PATH" GO111MODULE=off make -C "$BENCH" init deps build
test -x "$BENCH/bin/bench"

# ---- 外部API(isubank・isulogger) ------------------------------------------------------------
# 上流の blackbox/docker-compose.yml は起動のたびに golang:1.11 の中で `go get ./...` し、依存の最新版を取るので
# 今は Go 1.11 でビルドできない。依存を当時の版に固定してビルド時にバイナリにする。
BB="$HOME_DIR/blackbox"
sudo mkdir -p "$BB/gopath/src/github.com/go-sql-driver" "$BB/gopath/src/github.com/pkg" "$BB/bin"
sudo cp -a "$SRC/blackbox/bank" "$SRC/blackbox/logger" "$SRC/blackbox/sql" "$SRC/blackbox/conf.d" "$BB/"
sudo cp -a "$SRC/blackbox/bank" "$SRC/blackbox/logger" "$BB/gopath/src/"
# go-sql-driver/mysql は webapp の Gopkg.lock と同じリビジョン、pkg/errors は bench の Gopkg.toml と同じ v0.8.0
sudo git clone -q https://github.com/go-sql-driver/mysql "$BB/gopath/src/github.com/go-sql-driver/mysql"
sudo git -C "$BB/gopath/src/github.com/go-sql-driver/mysql" checkout -q d523deb1b23d913de5bdada721a6071e71283618
sudo git clone -q https://github.com/pkg/errors "$BB/gopath/src/github.com/pkg/errors"
sudo git -C "$BB/gopath/src/github.com/pkg/errors" checkout -q v0.8.0
sudo chown -R isucon:isucon "$BB"
sudo -u isucon -H sh -c "cd $BB/gopath/src/bank && GOPATH=$BB/gopath GO111MODULE=off $GO build -o $BB/bin/isubank ."
sudo -u isucon -H sh -c "cd $BB/gopath/src/logger && GOPATH=$BB/gopath GO111MODULE=off $GO build -o $BB/bin/isulogger ."

# isubank・isulogger は本番では nginx(openresty)が TLS を受けて裏に流していた。isuenv では証明書の問題を避けるため
# 平文HTTPで受ける。競技ノードのアプリには http://<ベンチノード>:5515(銀行)と :5516(ロガー)を渡す。
# ロガーのレート制限(Authorization ごとに 20r/s, burst 20, 同時接続 10。超えたら 429)は本番の nginx.conf と同じにする。
sudo tee "$BB/nginx.conf" >/dev/null <<'EOF'
worker_processes 2;
daemon off;
events {
    worker_connections  10240;
}
http {
    default_type  application/octet-stream;
    sendfile      on;
    error_log /dev/stdout info;
    access_log off;
    server_tokens off;
    client_max_body_size 10m;

    limit_conn_zone $http_authorization zone=conn_logger:10m;
    limit_req_zone  $http_authorization zone=req_logger:10m rate=20r/s;

    upstream isubank {
      server 127.0.0.1:15515;
      keepalive 32;
    }

    upstream logger {
      server 127.0.0.1:15516;
      keepalive 32;
    }

    server {
      listen 5515;
      location / {
        proxy_set_header Connection "";
        proxy_http_version 1.1;
        proxy_pass http://isubank;
      }
    }

    server {
      listen 5516;
      location / {
        limit_conn conn_logger 10;
        limit_req zone=req_logger burst=20 nodelay;
        limit_req_status 429;
        limit_conn_status 429;

        proxy_request_buffering off;
        proxy_set_header Connection "";
        proxy_http_version 1.1;
        proxy_pass http://logger;
      }
    }
}
EOF

sudo tee "$BB/docker-compose.yml" >/dev/null <<EOF
# isuenv: 上流の blackbox/docker-compose.yml から、isubank・logger をホストのバイナリ(systemd)に出したもの
services:
  nginx:
    image: openresty/openresty:1.13.6.2-alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    network_mode: host
    command:
      - "/usr/local/openresty/bin/openresty"
      - "-c"
      - "/etc/nginx/nginx.conf"
    restart: always

  mysql:
    image: $MYSQL_IMAGE
    network_mode: host
    environment:
      - "TZ=Asia/Tokyo"
      - "MYSQL_ROOT_PASSWORD=root"
    volumes:
      - mysql:/var/lib/mysql
      - ./sql:/docker-entrypoint-initdb.d
      - ./conf.d:/etc/mysql/conf.d
    restart: always

volumes:
  mysql:
EOF
sudo chown -R isucon:isucon "$BB"

sudo tee /etc/systemd/system/isuenv-blackbox.service >/dev/null <<'EOF'
[Unit]
Description = isucon8-final external APIs (mysql + nginx for isubank / isulogger)
After = docker.service
Requires = docker.service

[Service]
WorkingDirectory=/home/isucon/blackbox
ExecStart = /usr/local/bin/docker-compose up
ExecStop = /usr/local/bin/docker-compose down
Restart = always
User = isucon
Group = isucon

[Install]
WantedBy = multi-user.target
EOF

sudo tee /etc/systemd/system/isuenv-isubank.service >/dev/null <<'EOF'
[Unit]
Description = isucon8-final isubank
After = isuenv-blackbox.service

[Service]
ExecStart = /home/isucon/blackbox/bin/isubank -port=15515 -dbhost=127.0.0.1 -dbuser=root -dbpass=root
Restart = always
RestartSec = 2
User = isucon
Group = isucon
LimitNOFILE = 102400

[Install]
WantedBy = multi-user.target
EOF

sudo tee /etc/systemd/system/isuenv-isulogger.service >/dev/null <<'EOF'
[Unit]
Description = isucon8-final isulogger

[Service]
ExecStart = /home/isucon/blackbox/bin/isulogger -port=15516
Restart = always
RestartSec = 2
User = isucon
Group = isucon
LimitNOFILE = 102400

[Install]
WantedBy = multi-user.target
EOF

sudo docker pull -q openresty/openresty:1.13.6.2-alpine
sudo systemctl daemon-reload
# 銀行の初期データ(z_isubankdata.sql.gz)の投入もビルド時に済ませる
sudo systemctl start isuenv-blackbox.service isuenv-isubank.service isuenv-isulogger.service
for i in $(seq 1 60); do
  if mysql_initialized blackbox-mysql-1 && curl -s -o /dev/null http://127.0.0.1:5515/ && curl -s -o /dev/null http://127.0.0.1:5516/; then
    echo "blackbox is up"
    break
  fi
  if [ "$i" -eq 60 ]; then
    sudo journalctl -u isuenv-blackbox.service -u isuenv-isubank.service -u isuenv-isulogger.service --no-pager | tail -50
    exit 1
  fi
  sleep 10
done
sudo systemctl stop isuenv-isubank.service isuenv-isulogger.service isuenv-blackbox.service
# 外部APIは既定では止めておき、isuenv-node-init がベンチを動かすノードでだけ有効にする

# ---- 起動時のノードごとの切り替え ---------------------------------------------------------------
# isuenv の user-data が /etc/isuenv/node.env(ISUENV_ROLE=app|bench, ISUENV_NODE_INDEX, ISUENV_NODES, ISUENV_HAS_BENCH)を
# 書いてからこれを実行する。何度実行しても同じ結果になるようにする(メモリ制限の再起動後に再実行されうる)。
sudo tee /usr/local/sbin/isuenv-node-init >/dev/null <<'EOF'
#!/bin/sh
# isucon8-final: ベンチを動かすノードで外部APIを、競技ノードでアプリを動かす。
#   ベンチノード                       : 外部APIだけ(アプリは止める)
#   ベンチノードが無い環境の1号機      : アプリと外部APIの両方(ベンチも1号機で回すため)
#   それ以外の競技ノード               : アプリだけ(本番の初期状態は全台に同じアプリ)
set -e
. /etc/isuenv/node.env
blackbox="isuenv-blackbox.service isuenv-isubank.service isuenv-isulogger.service"
if [ "$ISUENV_ROLE" = bench ]; then
  systemctl disable --now isucoin.service
  systemctl enable --now $blackbox
elif [ "$ISUENV_NODE_INDEX" = 1 ] && [ "$ISUENV_HAS_BENCH" != 1 ]; then
  systemctl enable --now $blackbox
else
  systemctl disable --now $blackbox
fi
EOF
sudo chmod 755 /usr/local/sbin/isuenv-node-init

# ---- 後始末 -------------------------------------------------------------------------------------
sudo rm -rf "$SRC" "$BB/gopath/pkg"
sudo apt-get clean
sudo sh -c 'rm -rf /var/lib/apt/lists/*'
