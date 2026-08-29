package cmd

import (
	"strings"
	"testing"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
)

// ベンチ専用ノードがある構成では、そのノードで打つ。競技ノードでベンチを回すと
// 負荷生成がアプリのCPUを食ってスコアが正しく測れない。
func TestBenchTargetPrefersBenchNode(t *testing.T) {
	env := engine.Env{Name: "isucon14", Nodes: []engine.Node{
		{Index: 1, PrivateIP: "10.100.0.5", Role: engine.RoleApp},
		{Index: 2, PrivateIP: "10.100.0.6", Role: engine.RoleApp},
		{Index: 3, PrivateIP: "10.100.0.7", Role: engine.RoleApp},
		{Index: 4, PrivateIP: "10.100.0.9", Role: engine.RoleBench},
	}}
	idx, vars, err := benchTarget(env)
	if err != nil {
		t.Fatal(err)
	}
	if idx != 4 {
		t.Errorf("node index = %d, want 4 (the bench node)", idx)
	}
	if vars.BenchIP != "10.100.0.9" {
		t.Errorf("BenchIP = %q, want the bench node's own IP", vars.BenchIP)
	}
	// ベンチ対象は1号機。どこを叩くかはUIが無い以上ここで決め打つ。
	if vars.TargetIP != "10.100.0.5" {
		t.Errorf("TargetIP = %q, want 10.100.0.5", vars.TargetIP)
	}
	// ALL_ADDRESSES に相当する値にはベンチノードを混ぜない。
	if vars.AllIPs != "10.100.0.5,10.100.0.6,10.100.0.7" {
		t.Errorf("AllIPs = %q", vars.AllIPs)
	}
}

// ベンチノードが無い構成(private-isu の1台完結など)では1号機で打つ。
// このとき対象も自分自身になる。
func TestBenchTargetFallsBackToFirstNode(t *testing.T) {
	env := engine.Env{Name: "private-isu", Nodes: []engine.Node{
		{Index: 1, PrivateIP: "10.100.0.1", Role: engine.RoleApp},
	}}
	idx, vars, err := benchTarget(env)
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Errorf("node index = %d, want 1", idx)
	}
	if vars.TargetIP != "10.100.0.1" || vars.BenchIP != "10.100.0.1" {
		t.Errorf("vars = %+v", vars)
	}
}

// isuenv:role タグを持たない古いインスタンスは競技ノードとして扱う(既存の慣習に合わせる)。
func TestBenchTargetTreatsEmptyRoleAsApp(t *testing.T) {
	env := engine.Env{Name: "isucon13", Nodes: []engine.Node{
		{Index: 1, PrivateIP: "10.100.0.1"},
		{Index: 2, PrivateIP: "10.100.0.2"},
	}}
	_, vars, err := benchTarget(env)
	if err != nil {
		t.Fatal(err)
	}
	if vars.AllIPs != "10.100.0.1,10.100.0.2" {
		t.Errorf("AllIPs = %q", vars.AllIPs)
	}
}

func TestBenchTargetEmptyEnv(t *testing.T) {
	if _, _, err := benchTarget(engine.Env{Name: "isucon14"}); err == nil {
		t.Fatal("want error for an env with no nodes")
	}
}

func TestBenchCommandLine(t *testing.T) {
	b := catalog.Bench{
		User:    "isucon",
		Workdir: "/home/isucon",
		Command: "./bench",
		Args:    []string{"run", "--addr", "{{.TargetIP}}:443"},
	}
	got, err := benchCommandLine("isucon14", 4, b, catalog.BenchVars{TargetIP: "10.100.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	want := "ssh isucon14-4 'cd /home/isucon && sudo -u isucon ./bench run --addr 10.100.0.5:443'"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// workdir と user が空なら cd と sudo を出さない。
func TestBenchCommandLineMinimal(t *testing.T) {
	b := catalog.Bench{Command: "/usr/local/bin/bench", Args: []string{"-t", "http://{{.TargetIP}}"}}
	got, err := benchCommandLine("x", 1, b, catalog.BenchVars{TargetIP: "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	want := "ssh x-1 '/usr/local/bin/bench -t http://10.0.0.1'"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// 出力はそのまま貼れる必要があるので、シングルクォートを含む引数は
// シェルとして壊れない形にエスケープする。
func TestBenchCommandLineQuotesSingleQuote(t *testing.T) {
	b := catalog.Bench{Command: "bench", Args: []string{"--name", "it's"}}
	got, err := benchCommandLine("x", 1, b, catalog.BenchVars{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "it's'") && !strings.Contains(got, `'\''`) {
		t.Errorf("single quote is not escaped: %s", got)
	}
}

// Bench が無い問題は、何を見ればいいかが分かるエラーにする。
func TestBenchOrErrorUnsupported(t *testing.T) {
	_, err := benchOrError("isucon13")
	if err == nil {
		t.Fatal("want error for a problem without a bench block")
	}
	if !strings.Contains(err.Error(), "isucon13") {
		t.Errorf("error should name the problem: %v", err)
	}
	if !strings.Contains(err.Error(), "isuenv problems") {
		t.Errorf("error should point at where to look: %v", err)
	}
}

func TestBenchOrErrorSupported(t *testing.T) {
	for _, name := range []string{"isucon14", "private-isu"} {
		if _, err := benchOrError(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
