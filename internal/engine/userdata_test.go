package engine

import (
	"strings"
	"testing"
	"time"
)

func TestBuildUserData(t *testing.T) {
	ud := BuildUserData(8 * time.Hour)
	if !strings.HasPrefix(ud, "#!/bin/sh\n") {
		t.Errorf("user data must be a shell script: %q", ud)
	}
	if !strings.Contains(ud, "shutdown -P +480") {
		t.Errorf("expected shutdown after 480 minutes: %q", ud)
	}
}

func TestBuildUserData_MinimumOneMinute(t *testing.T) {
	ud := BuildUserData(10 * time.Second)
	if !strings.Contains(ud, "shutdown -P +1") {
		t.Errorf("TTL below 1 minute must clamp to 1: %q", ud)
	}
}
