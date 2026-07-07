package engine

import (
	"fmt"
	"time"
)

// BuildUserData は絶対時刻expiresAtを過ぎたらインスタンス自身がシャットダウンするuser-dataを返す。
// RunInstances側で instance-initiated-shutdown-behavior=terminate と組み合わせることで
// CLIが動いていなくても環境が自己消滅する。
//
// `shutdown -P +N`（相対時間指定）はreboot時にキャンセルされ、かつuser-dataは初回起動時にしか
// 実行されないため、リブートされた練習環境は永久に動き続けてしまう。そこで絶対時刻を
// /var/lib/isuenv-expires-at に書き込み、cron.dで毎分チェックする方式にすることでリブート耐性を持たせる。
func BuildUserData(expiresAt time.Time) string {
	epoch := expiresAt.Unix()
	return fmt.Sprintf(`#!/bin/sh
echo %d > /var/lib/isuenv-expires-at
cat <<'CRON' > /etc/cron.d/isuenv-ttl
* * * * * root [ "$(date +\%%s)" -ge "$(cat /var/lib/isuenv-expires-at)" ] && /sbin/shutdown -P now "isuenv TTL expired"
CRON
`, epoch)
}
