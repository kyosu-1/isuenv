package engine

import (
	"fmt"
	"strings"
	"time"
)

// BuildUserData は絶対時刻expiresAtを過ぎたらインスタンス自身がシャットダウンするuser-dataを返す。
// RunInstances側で instance-initiated-shutdown-behavior=terminate と組み合わせることで
// CLIが動いていなくても環境が自己消滅する。
//
// `shutdown -P +N`（相対時間指定）はreboot時にキャンセルされ、かつuser-dataは初回起動時にしか
// 実行されないため、リブートされた練習環境は永久に動き続けてしまう。そこで絶対時刻を
// /var/lib/isuenv-expires-at に書き込み、cron.dで毎分チェックする方式にすることでリブート耐性を持たせる。
//
// 続けてノードの情報(nodeInfoScript)を書き、AMIが初期化スクリプトを持っていれば実行する。
// memGB が正なら、最後にカーネル引数 mem= でOSから見えるメモリを約 memGB GiB に絞る処理(memLimitScript)を足す。
// TTLの設定を先に書くので、メモリ制限の適用(再起動を伴う)がどう転んでも自己消滅は効く。
func BuildUserData(expiresAt time.Time, memGB int, node NodeInfo) string {
	epoch := expiresAt.Unix()
	script := fmt.Sprintf(`#!/bin/sh
echo %d > /var/lib/isuenv-expires-at
cat <<'CRON' > /etc/cron.d/isuenv-ttl
* * * * * root [ "$(date +\%%s)" -ge "$(cat /var/lib/isuenv-expires-at)" ] && /sbin/shutdown -P now "isuenv TTL expired"
CRON
`, epoch)
	script += node.script()
	if memGB > 0 {
		script += strings.NewReplacer(
			"@MEM@", fmt.Sprintf("%dG", memKernelArgGB(memGB)),
			"@MEM_KB@", fmt.Sprintf("%d", memGB*1024*1024),
		).Replace(memLimitScript)
	}
	return script
}

// NodeInfo は環境の中でのノードの位置づけ。AMIに渡し、ノードごとに初期状態を変える問題
// (ベンチノードでだけ外部APIを動かす isucon8-final など)が使う。
type NodeInfo struct {
	Role     string // RoleApp または RoleBench
	Index    int    // isuenv:node タグと同じ番号。競技ノードは1から、ベンチノードは競技ノードの次
	Nodes    int    // 競技ノードの台数
	HasBench bool   // ベンチノードがあるか
}

// script はノードの情報を /etc/isuenv/node.env に書き、AMIに /usr/local/sbin/isuenv-node-init が
// あれば実行するシェルを返す。問題ごとの切り替えはAMI側に置き、isuenv は事実を渡すだけにする。
// 初期化スクリプトを持たないAMI(matsuu/aws-isucon のもの)では、ファイルを書くだけで何も起きない。
// メモリ制限の再起動の後で user-data がもう一度実行されることがあるので、初期化スクリプトは冪等に作る。
// 初期化に失敗しても、続くメモリ制限は適用する。
func (n NodeInfo) script() string {
	hasBench := 0
	if n.HasBench {
		hasBench = 1
	}
	return fmt.Sprintf(`mkdir -p /etc/isuenv
cat <<'NODE' > /etc/isuenv/node.env
ISUENV_ROLE=%s
ISUENV_NODE_INDEX=%d
ISUENV_NODES=%d
ISUENV_HAS_BENCH=%d
NODE
if [ -x /usr/local/sbin/isuenv-node-init ]; then
  /usr/local/sbin/isuenv-node-init || echo "isuenv: isuenv-node-init failed"
fi
`, n.Role, n.Index, n.Nodes, hasBench)
}

// memKernelArgGB は、OSから見えるメモリを memGB にするためにカーネル引数 mem= へ渡す値を返す。
// mem= は「使う物理アドレスの上限」で、EC2(Nitro)のメモリは 0〜3GiB と 4GiB 以降に分かれて
// 載っている(3〜4GiB はデバイス用の穴)。上限が 3GiB を超えると穴の 1GiB ぶん使える量が減るので、
// その分を足す。実機: c5.2xlarge で mem=8G だと free は 6924MB、c5.large で mem=2G は 1913MB。
func memKernelArgGB(memGB int) int {
	if memGB > 3 {
		return memGB + 1
	}
	return memGB
}

// memLimitScript はカーネル引数 mem= でOSから見えるメモリを絞る。上流の provisioning が
// 本番のメモリ量を再現するのに使っているのと同じ方法(grub の mem=1G / mem=2G)。
// カーネル引数は起動時にしか効かないので、grub の設定を書き換えて1回だけ再起動する。
//
//   - 既に同じ値で起動していれば何もしない。AMIが同じ mem= を焼き込んでいる場合と、
//     再起動後にこのスクリプトがもう一度走った場合(cloud-init は完走前に再起動されると
//     次の起動でuser-dataを再実行することがある)の両方をこれで受ける。
//   - 搭載メモリが制限値以下なら何もしない。タイプを明示指定されてもメモリ制限は残すので、
//     制限値と同じかそれより小さいタイプ(例: mem=8G に対して 8GiB の c5.xlarge)が来うる。
//     絞るものが無いのに再起動だけするのを避ける。
//   - 一度適用を試みたら印(/var/lib/isuenv-mem-applied)を残し、二度目は再起動しない。
//     grub の変更が何かの理由で効かなかったときに再起動を繰り返さないため。
//   - x86 のカーネルは mem= が複数あると小さいほうが効く。AMIが別の値を焼き込んでいても
//     意図した値になるよう、既存の mem= は取り除いてから足す(二重指定にしない)。
//     grub.d のファイルは名前順に読まれるので、最後に読まれる名前にして他の設定の後で書き換える。
//   - grub の設定の入れ方はディストリビューションで違う。Debian系(update-grub)は grub.d に
//     ファイルを置く。RHEL系(grub2-mkconfig。isucon8-qualify のAMIが CentOS 7)は grub.d を
//     読まないので /etc/default/grub の末尾に足す(このファイルはシェルとして読まれるので、
//     後ろに書いた代入が勝つ)。
//   - grub の更新が失敗したら再起動しない(制限なしのまま動き続ける)。
//   - ここでの再起動はOS内の reboot なので instance-initiated-shutdown-behavior=terminate の
//     対象にならず、インスタンスもIPもそのまま残る。
//
// @MEM@ は "2G" のような値に、@MEM_KB@ は同じ量のkB数(/proc/meminfo の単位)に置換される。
const memLimitScript = `
ISUENV_MEM=@MEM@
if grep -Eq "(^| )mem=${ISUENV_MEM}( |\$)" /proc/cmdline; then
  echo "isuenv: mem=${ISUENV_MEM} is already in effect"
elif [ "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" -le @MEM_KB@ ]; then
  echo "isuenv: memory is already at or below ${ISUENV_MEM}; nothing to limit"
elif [ -e /var/lib/isuenv-mem-applied ]; then
  echo "isuenv: mem=${ISUENV_MEM} was already applied once but is not in effect; not rebooting again"
else
  isuenv_apply_grub() {
    if command -v update-grub >/dev/null 2>&1; then
      mkdir -p /etc/default/grub.d
      cat <<'GRUB' > /etc/default/grub.d/zz-isuenv-mem.cfg
# isuenv: limit memory to the official spec of the contest (kernel argument mem=@MEM@).
GRUB_CMDLINE_LINUX="$(printf '%s' "$GRUB_CMDLINE_LINUX" | sed -E 's/(^| +)mem=[^ ]*//g')"
GRUB_CMDLINE_LINUX_DEFAULT="$(printf '%s' "$GRUB_CMDLINE_LINUX_DEFAULT" | sed -E 's/(^| +)mem=[^ ]*//g') mem=@MEM@"
GRUB
      update-grub
    elif command -v grub2-mkconfig >/dev/null 2>&1; then
      cat <<'GRUB' >> /etc/default/grub
# isuenv: limit memory to the official spec of the contest (kernel argument mem=@MEM@).
GRUB_CMDLINE_LINUX="$(printf '%s' "$GRUB_CMDLINE_LINUX" | sed -E 's/(^| +)mem=[^ ]*//g') mem=@MEM@"
GRUB
      grub2-mkconfig -o /boot/grub2/grub.cfg
    else
      echo "isuenv: neither update-grub nor grub2-mkconfig is available"
      return 1
    fi
  }
  if isuenv_apply_grub; then
    echo "${ISUENV_MEM}" > /var/lib/isuenv-mem-applied
    shutdown -r now "isuenv: rebooting once to apply mem=${ISUENV_MEM}"
  else
    echo "isuenv: updating grub failed; memory is not limited"
  fi
fi
`
