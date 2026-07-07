package sshconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	got := Render([]Host{
		{Alias: "isucon13-1", HostName: "54.0.0.1", User: "ubuntu", IdentityFile: "/home/u/.ssh/isuenv.pem"},
	})
	for _, want := range []string{
		"Host isucon13-1",
		"HostName 54.0.0.1",
		"User ubuntu",
		"IdentityFile /home/u/.ssh/isuenv.pem",
		"StrictHostKeyChecking no",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered config should contain %q:\n%s", want, got)
		}
	}
}

func TestEnsureInclude_Idempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("Host example\n  HostName example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := EnsureInclude(configPath, "~/.ssh/isuenv_config"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	data, _ := os.ReadFile(configPath)
	if got := strings.Count(string(data), "Include ~/.ssh/isuenv_config"); got != 1 {
		t.Errorf("Include line must appear exactly once, got %d:\n%s", got, data)
	}
	if !strings.Contains(string(data), "Host example") {
		t.Error("existing content must be preserved")
	}
}

func TestEnsureInclude_CreatesConfigIfMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config")
	if err := EnsureInclude(configPath, "~/.ssh/isuenv_config"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config must be created: %v", err)
	}
	if !strings.Contains(string(data), "Include ~/.ssh/isuenv_config") {
		t.Errorf("include line missing:\n%s", data)
	}
}
