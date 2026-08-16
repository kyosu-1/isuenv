package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionStringUsesInjectedValues(t *testing.T) {
	restore := func(v, c, d string) { version, commit, date = v, c, d }
	defer restore(version, commit, date)

	version, commit, date = "v1.2.3", "abc1234", "2026-08-16T00:00:00Z"

	got := versionString()
	want := "v1.2.3 (abc1234, 2026-08-16T00:00:00Z)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

// ldflagsで注入されなかった場合(go build等)はモジュールのビルド情報にフォールバックする。
// テストバイナリのビルド情報は "(devel)" などになるため、既定値に戻ることだけを確認する。
func TestVersionStringFallsBackWhenNotInjected(t *testing.T) {
	restore := func(v string) { version = v }
	defer restore(version)

	version = "dev"

	if got := versionString(); got == "" {
		t.Error("versionString() must not be empty when version is not injected")
	}
}

func TestRenderVersion(t *testing.T) {
	var buf bytes.Buffer
	renderVersion(&buf)

	out := buf.String()
	if !strings.HasPrefix(out, "isuenv ") {
		t.Errorf("output should start with %q:\n%s", "isuenv ", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output should end with a newline:\n%q", out)
	}
}
