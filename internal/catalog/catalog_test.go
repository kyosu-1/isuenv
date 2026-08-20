package catalog

import (
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	problems := List()
	if len(problems) < 10 {
		t.Fatalf("expected at least 10 problems, got %d", len(problems))
	}
	for _, p := range problems {
		// AMIの提供元は問題ごとに異なる(matsuu/aws-isucon と private-isu)ため、
		// owner idは固定値ではなく埋まっていることだけを検証する。
		if p.Name == "" || p.AMIPattern == "" || p.OwnerID == "" || p.SSHUser == "" {
			t.Errorf("problem has empty required field: %+v", p)
		}
		if p.InstanceType == "" {
			t.Errorf("problem %s: instance type should have been defaulted by List()", p.Name)
		}
	}
}

// instance_type を省略した問題は List() が DefaultInstanceType で埋める。
func TestListDefaultsInstanceType(t *testing.T) {
	p, err := Lookup("isucon13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.InstanceType != DefaultInstanceType {
		t.Errorf("instance type = %q, want %q", p.InstanceType, DefaultInstanceType)
	}
}

func TestLookup(t *testing.T) {
	p, err := Lookup("isucon13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.AMIPattern != "isucon13-*" {
		t.Errorf("unexpected ami pattern: %q", p.AMIPattern)
	}

	_, err = Lookup("no-such-problem")
	if err == nil {
		t.Fatal("expected error for unknown problem")
	}
	if !strings.Contains(err.Error(), "isuenv problems") {
		t.Errorf("error should mention `isuenv problems`: %v", err)
	}
}

// private-isu は matsuu/aws-isucon とは別アカウントのAMIで、推奨インスタンスタイプも異なる。
func TestLookupPrivateISU(t *testing.T) {
	p, err := Lookup("private-isu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.AMIPattern != "catatsuy_private_isu_amd64_*" {
		t.Errorf("unexpected ami pattern: %q", p.AMIPattern)
	}
	if p.OwnerID != "459514135530" {
		t.Errorf("unexpected owner id: %q", p.OwnerID)
	}
	if p.SSHUser != "ubuntu" {
		t.Errorf("unexpected ssh user: %q", p.SSHUser)
	}
	if p.InstanceType != "c7a.large" {
		t.Errorf("instance type = %q, want c7a.large", p.InstanceType)
	}
	// 上流READMEの推奨(競技 c7a.large / ベンチ c7a.xlarge)。ベンチ側が先に飽和すると
	// スコアが負荷生成側の性能で頭打ちになるため、ベンチは1段上のタイプにする。
	if p.BenchInstanceType != "c7a.xlarge" {
		t.Errorf("bench instance type = %q, want c7a.xlarge", p.BenchInstanceType)
	}
}

// ベンチ用タイプは推奨値の根拠がある問題にだけ設定する。無根拠な既定値を配らないため、
// private-isu 以外は空のままであることを確かめる。
func TestBenchInstanceTypeOnlyWhereRecommended(t *testing.T) {
	for _, p := range List() {
		if p.Name == "private-isu" {
			continue
		}
		if p.BenchInstanceType != "" {
			t.Errorf("problem %s should not have a bench instance type: %q", p.Name, p.BenchInstanceType)
		}
	}
}
