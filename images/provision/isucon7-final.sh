#!/bin/bash
# isucon7-final(ISUCON7 本戦)のAMIを作る。
#
# 本番は Ubuntu 16.04 の上で各言語を xbuild でソースからビルドしていた(上流README)。isuenv では
# Ubuntu 24.04 の上に Go 実装だけを入れる(当時の言語処理系は今の OpenSSL などでビルドが通らないため)。
#
#   /home/isucon/webapp   アプリ。Go 実装を cco.golang.service で動かす(本番と同じユニット)
#   /home/isucon/db       DB の初期化スクリプト(マニュアル「MySQLのリカバリ方法」の ~/db)
#   /home/isucon/env.sh   アプリの DB 接続先(cco.golang.service の EnvironmentFile)
#   /home/isucon/bench    ベンチマーカー
#
# 本番の初期構成は「先頭の3台がWebサーバー、残りの1台がDBサーバー」で、各サーバーの /etc/hosts に
# 4台の短いホスト名とローカルIPが書かれ、アプリはそのホスト名で DB につなぐ(マニュアル)。
# どの台が何をするかは起動時に /usr/local/sbin/isuenv-node-init が決める(このファイルの最後)。
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive

# 2026-10-24 時点の isucon/isucon7-final の master
REPO_URL=https://github.com/isucon/isucon7-final
REPO_REV=c9b568348fac818cadb698a06f41bcc07e1ab66c
# 本番と同じ版(上流README の xbuild/go-install 1.9.2)
GO_VERSION=1.9.2
DEP_VERSION=v0.5.4

HOME_DIR=/home/isucon
SRC=/tmp/isucon7-final

sudo apt-get update
# 本番は 16.04 の apt の MySQL 5.7。24.04 の apt は 8.0 なので、アプリ用ユーザーを当時の認証方式
# (mysql_native_password。アプリの go-sql-driver/mysql v1.3 は 8.0 既定の caching_sha2_password に対応していない)で作る。
sudo apt-get install -y --no-install-recommends mysql-server nginx git make curl ca-certificates

sudo useradd -m -s /bin/bash isucon
echo 'isucon ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/isucon
sudo chmod 755 "$HOME_DIR"

git clone "$REPO_URL" "$SRC"
git -C "$SRC" checkout -q "$REPO_REV"

sudo cp -a "$SRC/webapp" "$HOME_DIR/webapp"
sudo cp -a "$SRC/db" "$HOME_DIR/db"
sudo cp -a "$SRC/bench" "$HOME_DIR/bench"
sudo cp -a "$SRC/files" "$HOME_DIR/files"
sudo chown -R isucon:isucon "$HOME_DIR"

# ---- Go(本番は ~/local/go に xbuild で入れていた) ----------------------------------------------
sudo -u isucon mkdir -p "$HOME_DIR/local"
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | sudo -u isucon tar -C "$HOME_DIR/local" -xz
sudo -u isucon mkdir -p "$HOME_DIR/go/bin"
sudo -u isucon curl -fsSL -o "$HOME_DIR/go/bin/dep" "https://github.com/golang/dep/releases/download/${DEP_VERSION}/dep-linux-amd64"
sudo chmod 755 "$HOME_DIR/go/bin/dep"
# 上流README の .bashrc の設定(Go の分だけ)
sudo -u isucon tee -a "$HOME_DIR/.bashrc" >/dev/null <<'EOF'

export PATH=$HOME/local/go/bin:$HOME/go/bin:$PATH
EOF
GOENV="PATH=$HOME_DIR/local/go/bin:$HOME_DIR/go/bin:/usr/bin:/bin GO111MODULE=off"

# ---- MySQL ------------------------------------------------------------------------------------
# 他のノード(Web)から DB ノードにつなぐので、全インターフェースで待ち受ける。
# アプリの go-sql-driver/mysql v1.3 は、サーバーの既定の認証方式(8.0 は caching_sha2_password)から
# mysql_native_password に切り替える要求を受けると「this user requires mysql native password authentication」で
# 接続を拒む。サーバーの既定を 5.7 と同じ mysql_native_password にして、最初からそれで認証させる。
sudo tee /etc/mysql/mysql.conf.d/zz-isuenv.cnf >/dev/null <<'EOF'
[mysqld]
bind-address = 0.0.0.0
mysqlx-bind-address = 127.0.0.1
default_authentication_plugin = mysql_native_password
EOF
sudo systemctl restart mysql
sudo "$HOME_DIR/db/init.sh"
sudo mysql -e "CREATE USER 'isucon'@'%' IDENTIFIED WITH mysql_native_password BY 'isucon'; GRANT ALL ON isudb.* TO 'isucon'@'%';"

# 1台構成ではローカルの DB につなぐ。複数台構成では isuenv-node-init が DB ノードのホスト名に書き換える。
sudo -u isucon tee "$HOME_DIR/env.sh" >/dev/null <<'EOF'
ISU_DB_HOST=127.0.0.1
ISU_DB_PORT=3306
ISU_DB_USER=isucon
ISU_DB_PASSWORD=isucon
EOF

# ---- アプリ(Go) -------------------------------------------------------------------------------
# Makefile は GOPATH に $PWD を使うので、make -C ではなく cd してから呼ぶ
sudo -u isucon -H env $GOENV sh -c "cd $HOME_DIR/webapp/go && make ensure build"
test -x "$HOME_DIR/webapp/go/app"
sudo cp "$HOME_DIR/files/cco.golang.service" /etc/systemd/system/

# nginx(上流 files/cco.nginx.conf をそのまま使う。root は /home/isucon/webapp/public)
sudo cp "$HOME_DIR/files/cco.nginx.conf" /etc/nginx/sites-available/cco.nginx.conf
sudo rm -f /etc/nginx/sites-enabled/*
sudo ln -s ../sites-available/cco.nginx.conf /etc/nginx/sites-enabled/cco.nginx.conf
sudo nginx -t

sudo systemctl daemon-reload
sudo systemctl enable --now cco.golang.service
sudo systemctl restart nginx
for i in $(seq 1 30); do
  if curl -s -o /dev/null -w '%{http_code}' http://localhost/ | grep -q '^200$' \
    && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost/initialize | grep -q '^20'; then
    echo "app is up"
    break
  fi
  if [ "$i" -eq 30 ]; then
    sudo journalctl -u cco.golang.service --no-pager | tail -30
    exit 1
  fi
  sleep 2
done

# ---- ベンチマーカー ----------------------------------------------------------------------------
sudo -u isucon -H env $GOENV sh -c "cd $HOME_DIR/bench && make ensure build"
test -x "$HOME_DIR/bench/bench"

# ---- 起動時のノードごとの切り替え ---------------------------------------------------------------
# isuenv の user-data が /etc/isuenv/node.env(ISUENV_ROLE=app|bench, ISUENV_NODE_INDEX, ISUENV_NODES, ISUENV_HAS_BENCH)を
# 書いてからこれを実行する。全ノードのプライベートIPは isuenv がインスタンスのタグ isuenv:hosts
# ("1=10.100.0.5,2=10.100.0.6,...,bench=10.100.0.9")に付けるので、インスタンスメタデータから読む。
# 何度実行しても同じ結果になるようにする(メモリ制限の再起動後に再実行されうる)。
sudo tee /usr/local/sbin/isuenv-node-init >/dev/null <<'EOF'
#!/bin/sh
# isucon7-final:
#   競技ノード4台以上   1〜3号機が Web(nginx + アプリ)、4号機が DB(MySQL)。本番の初期構成と同じ。
#                       5号機以降は Web と同じ状態で起動する(アプリの DB は4号機)。
#                       /etc/hosts に isu1〜isuN(とベンチノード isu-bench)を書き、アプリは isu4 の DB につなぐ。
#   競技ノード1〜3台    各ノードが1台で完結する(Web と DB を両方動かす)
#   ベンチノード        アプリも DB も止める(ベンチだけ)
set -e
. /etc/isuenv/node.env

web="nginx.service cco.golang.service"
db="mysql.service"

hosts_tag() {
  token=$(curl -s -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 300" http://169.254.169.254/latest/api/token)
  curl -s -f -H "X-aws-ec2-metadata-token: $token" http://169.254.169.254/latest/meta-data/tags/instance/isuenv:hosts
}

if [ "$ISUENV_ROLE" = bench ]; then
  systemctl disable --now $web $db
  exit 0
fi

if [ "$ISUENV_NODES" -lt 4 ]; then
  systemctl enable --now $db $web
  exit 0
fi

# isuenv は全ノードを起動してからタグを付けるので、起動直後はまだ無いことがある。
# インスタンスメタデータへのタグの反映は数分かかることがある(2026-10-24 に4台中1台で数分かかった)。
hosts=""
for i in $(seq 1 180); do
  hosts=$(hosts_tag) && [ -n "$hosts" ] && break
  sleep 5
done
if [ -z "$hosts" ]; then
  echo "isuenv-node-init: tag isuenv:hosts is not available" >&2
  exit 1
fi

sed -i '/# isuenv-hosts$/d' /etc/hosts
echo "$hosts" | tr ',' '\n' | while IFS='=' read -r name ip; do
  if [ "$name" = bench ]; then
    echo "$ip isu-bench # isuenv-hosts" >> /etc/hosts
  else
    echo "$ip isu$name # isuenv-hosts" >> /etc/hosts
  fi
done
sed -i 's/^ISU_DB_HOST=.*/ISU_DB_HOST=isu4/' /home/isucon/env.sh

if [ "$ISUENV_NODE_INDEX" = 4 ]; then
  systemctl disable --now $web
  systemctl enable --now $db
else
  systemctl disable --now $db
  systemctl enable $web
  systemctl restart $web
fi
EOF
sudo chmod 755 /usr/local/sbin/isuenv-node-init

# ---- 後始末 -------------------------------------------------------------------------------------
sudo rm -rf "$SRC" "$HOME_DIR/go/pkg" "$HOME_DIR/.cache"
sudo apt-get clean
sudo sh -c 'rm -rf /var/lib/apt/lists/*'
