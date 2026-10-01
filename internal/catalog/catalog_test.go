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

// ベンチ用タイプは推奨値の根拠がある問題にだけ設定する。根拠は上流が明記した推奨タイプか、
// 本番のベンチのスペック(それを満たす最小のタイプに読み替える)。無根拠な既定値を配らないため、
// スペックが公開されていない isucon9-final は空のままであることを確かめる。
func TestBenchInstanceTypeOnlyWhereRecommended(t *testing.T) {
	want := map[string]string{
		"isucon9-qualify":  "c7a.xlarge",
		"isucon9-final":    "",
		"isucon10-qualify": "r5.large",
		"isucon10-final":   "c5.2xlarge",
		"isucon11-qualify": "c4.xlarge",
		"isucon11-final":   "c5.xlarge",
		"isucon12-qualify": "c5.xlarge",
		"isucon12-final":   "c5.xlarge",
		"isucon13":         "c5.2xlarge",
		"isucon14":         "c5.2xlarge",
		"private-isu":      "c7a.xlarge",
	}
	for _, p := range List() {
		w, ok := want[p.Name]
		if !ok {
			t.Errorf("problem %s is missing from this test; decide its bench instance type explicitly", p.Name)
			continue
		}
		if p.BenchInstanceType != w {
			t.Errorf("problem %s bench instance type = %q, want %q", p.Name, p.BenchInstanceType, w)
		}
	}
}

// ベンチの起動方法を持つ問題は Bench が非nil。v1で埋めるのは isucon14 と private-isu の2問。
// 「ベンチノードあり(isucon14)」と「専用ベンチバイナリ(private-isu)」の両パターンを踏むことで
// カタログの形が正しいかを検証できる。
func TestProblemsWithBench(t *testing.T) {
	want := map[string]bool{"isucon14": true, "private-isu": true}
	for _, p := range List() {
		if p.Bench == nil {
			if want[p.Name] {
				t.Errorf("problem %s should have a bench block", p.Name)
			}
			continue
		}
		if !want[p.Name] {
			t.Errorf("problem %s has an unexpected bench block", p.Name)
		}
		if p.Bench.User == "" || p.Bench.Command == "" || len(p.Bench.Args) == 0 {
			t.Errorf("problem %s has an incomplete bench block: %+v", p.Name, p.Bench)
		}
	}
}

func TestRenderArgs(t *testing.T) {
	b := Bench{Args: []string{
		"run",
		"--addr", "{{.TargetIP}}:443",
		"--payment-url", "http://{{.BenchIP}}:12346",
		"--all", "{{.AllIPs}}",
	}}
	got, err := b.RenderArgs(BenchVars{
		TargetIP: "10.100.0.5",
		BenchIP:  "10.100.0.9",
		AllIPs:   "10.100.0.5,10.100.0.6",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"run",
		"--addr", "10.100.0.5:443",
		"--payment-url", "http://10.100.0.9:12346",
		"--all", "10.100.0.5,10.100.0.6",
	}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

// 未知の変数はカタログのタイプミスなので、黙って空文字にせずエラーにする。
func TestRenderArgsUnknownVar(t *testing.T) {
	b := Bench{Args: []string{"{{.Nope}}"}}
	if _, err := b.RenderArgs(BenchVars{}); err == nil {
		t.Fatal("want error for an unknown template variable")
	}
}

// isucon14 のベンチはISUXBENCH_TARGETが無ければ --addr/--target をそのまま使う。
// 手で打つ経路ではsupervisorがいないので、両方を引数で渡す必要がある。
func TestIsucon14BenchArgs(t *testing.T) {
	p, err := Lookup("isucon14")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Bench.RenderArgs(BenchVars{TargetIP: "10.100.0.5", BenchIP: "10.100.0.9"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{
		"--addr 10.100.0.5:443",
		"--target https://isuride.xiv.isucon.net",
		"--payment-url http://10.100.0.9:12346",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q should contain %q", joined, want)
		}
	}
	// matsuu の README の `./bench run . run` は `go run . run` を置換した残骸で、
	// cobra が余分な位置引数を黙って捨てているだけ。カタログには持ち込まない。
	if strings.Contains(joined, "run . run") {
		t.Error("args should not carry the stray positional args from matsuu's README")
	}
}
