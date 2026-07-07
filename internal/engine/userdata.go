package engine

import (
	"fmt"
	"time"
)

// BuildUserData はTTL経過後にインスタンス自身がシャットダウンするuser-dataを返す。
// RunInstances側で instance-initiated-shutdown-behavior=terminate と組み合わせることで
// CLIが動いていなくても環境が自己消滅する。
func BuildUserData(ttl time.Duration) string {
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("#!/bin/sh\nshutdown -P +%d \"isuenv TTL expired\"\n", minutes)
}
