package engine

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildUserData(t *testing.T) {
	expiresAt := time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)
	ud := BuildUserData(expiresAt)
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
}
