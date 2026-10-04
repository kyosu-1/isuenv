package engine

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildUserData(t *testing.T) {
	expiresAt := time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)
	ud := BuildUserData(expiresAt, 0)
	if !strings.HasPrefix(ud, "#!/bin/sh\n") {
		t.Errorf("user data must be a shell script: %q", ud)
	}
	wantEpoch := strconv.FormatInt(expiresAt.Unix(), 10)
	if !strings.Contains(ud, wantEpoch) {
		t.Errorf("expected expiresAt epoch %s in user data: %q", wantEpoch, ud)
	}
	if !strings.Contains(ud, "/etc/cron.d/isuenv-ttl") {
		t.Errorf("expected cron.d install path: %q", ud)
	}
	if !strings.Contains(ud, "shutdown -P now") {
		t.Errorf("expected absolute-deadline shutdown (reboot-safe): %q", ud)
	}
	// メモリ制限が無ければ grub にも触らず、再起動もしない。
	for _, unwanted := range []string{"mem=", "update-grub", "shutdown -r"} {
		if strings.Contains(ud, unwanted) {
			t.Errorf("user data without a memory limit must not contain %q: %q", unwanted, ud)
		}
	}
}

func TestBuildUserDataMemLimit(t *testing.T) {
	expiresAt := time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)
	ud := BuildUserData(expiresAt, 2)

	// TTLの設定はメモリ制限(再起動を伴う)より前に書く。順序が逆だと、再起動で
	// スクリプトが打ち切られたときに自己消滅しない環境が残る。
	ttl := strings.Index(ud, "/etc/cron.d/isuenv-ttl")
	reboot := strings.Index(ud, "shutdown -r now")
	if ttl < 0 || reboot < 0 || ttl > reboot {
		t.Fatalf("TTL cron must be installed before the reboot (ttl at %d, reboot at %d): %q", ttl, reboot, ud)
	}
	if !strings.Contains(ud, strconv.FormatInt(expiresAt.Unix(), 10)) {
		t.Errorf("TTL expiry must be kept: %q", ud)
	}

	for _, want := range []string{
		// grub.d の最後に読まれる名前で書き、既存の mem= を取り除いてから足す(二重指定にしない)。
		"/etc/default/grub.d/zz-isuenv-mem.cfg",
		`sed -E 's/(^| +)mem=[^ ]*//g') mem=2G"`,
		"update-grub",
		// 既に同じ値で起動していれば何もしない(AMIが焼き込み済みの場合と、再起動後の再実行)。
		`grep -Eq "(^| )mem=${ISUENV_MEM}( |\$)" /proc/cmdline`,
		// 搭載メモリが制限値(2GiB = 2097152kB)以下なら、絞るものが無いので再起動しない。
		`-le 2097152 ]`,
		// 一度適用を試みたら二度目は再起動しない(再起動ループの防止)。
		"/var/lib/isuenv-mem-applied",
	} {
		if !strings.Contains(ud, want) {
			t.Errorf("user data should contain %q:\n%s", want, ud)
		}
	}
	if strings.Contains(ud, "@MEM") {
		t.Errorf("placeholder must be replaced: %q", ud)
	}
	// TTLの自己シャットダウン(-P)と違い、メモリ制限の適用は再起動(-r)。電源断にすると
	// instance-initiated-shutdown-behavior=terminate でインスタンスが消える。
	if strings.Count(ud, "shutdown -P") != 1 {
		t.Errorf("only the TTL cron may power off the instance: %q", ud)
	}

	if other := BuildUserData(expiresAt, 8); !strings.Contains(other, "ISUENV_MEM=9G") || !strings.Contains(other, "-le 8388608 ]") || strings.Contains(other, "2G") {
		t.Errorf("memory size must follow the argument: %q", other)
	}
}
