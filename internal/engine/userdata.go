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
// memGB が正なら、続けてカーネル引数 mem=<memGB>G でメモリを絞る処理(memLimitScript)を足す。
// TTLの設定を先に書くので、メモリ制限の適用(再起動を伴う)がどう転んでも自己消滅は効く。
func BuildUserData(expiresAt time.Time, memGB int) string {
	epoch := expiresAt.Unix()
	script := fmt.Sprintf(`#!/bin/sh
echo %d > /var/lib/isuenv-expires-at
cat <<'CRON' > /etc/cron.d/isuenv-ttl
* * * * * root [ "$(date +\%%s)" -ge "$(cat /var/lib/isuenv-expires-at)" ] && /sbin/shutdown -P now "isuenv TTL expired"
CRON
`, epoch)
	if memGB > 0 {
		script += strings.NewReplacer(
			"@MEM@", fmt.Sprintf("%dG", memGB),
			"@MEM_KB@", fmt.Sprintf("%d", memGB*1024*1024),
		).Replace(memLimitScript)
	}
	return script
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
//   - update-grub が失敗したら再起動しない(制限なしのまま動き続ける)。
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
  mkdir -p /etc/default/grub.d
  cat <<'GRUB' > /etc/default/grub.d/zz-isuenv-mem.cfg
# isuenv: limit memory to the official spec of the contest (kernel argument mem=@MEM@).
GRUB_CMDLINE_LINUX="$(printf '%s' "$GRUB_CMDLINE_LINUX" | sed -E 's/(^| +)mem=[^ ]*//g')"
GRUB_CMDLINE_LINUX_DEFAULT="$(printf '%s' "$GRUB_CMDLINE_LINUX_DEFAULT" | sed -E 's/(^| +)mem=[^ ]*//g') mem=@MEM@"
GRUB
  if update-grub; then
    echo "${ISUENV_MEM}" > /var/lib/isuenv-mem-applied
    shutdown -r now "isuenv: rebooting once to apply mem=${ISUENV_MEM}"
  else
    echo "isuenv: update-grub failed; memory is not limited"
  fi
fi
`
