package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
)

func TestRenderProblems(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	out := buf.String()
	for _, want := range []string{"NAME", "NODES", "TYPE", "BENCH TYPE", "BENCH CMD", "isucon13", "isucon14", "ubuntu", "private-isu", "c7a.large", "c7a.xlarge"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

// BENCH TYPE 列は推奨値のある問題だけタイプを出し、無い問題は "-" で埋める
// (列が空のままだとタブ区切りが潰れて読めなくなるため)。
func TestRenderProblemsBenchTypeColumn(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	// NAME / SSH USER / NODES / TYPE / BENCH TYPE の5列目を問題ごとに拾う。
	benchTypes := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 5 {
			benchTypes[fields[0]] = fields[4]
		}
	}
	// CPUやメモリを絞るベンチは、タイプに制限を添えて出す。
	if got := benchTypes["isucon9-qualify"]; got != "c5.4xlarge(12vCPU)" {
		t.Errorf("isucon9-qualify bench type = %q, want c5.4xlarge(12vCPU)", got)
	}
	if got := benchTypes["isucon13"]; got != "c5.2xlarge(mem=8G)" {
		t.Errorf("isucon13 bench type = %q, want c5.2xlarge(mem=8G)", got)
	}
	if got := benchTypes["private-isu"]; got != "c7a.xlarge" {
		t.Errorf("private-isu bench type = %q, want c7a.xlarge", got)
	}
	if got := benchTypes["isucon9-final"]; got != "-" {
		t.Errorf("isucon9-final has no recommended bench type, want a dash: got %q", got)
	}
}

// NODES 列は本番の台数、TYPE 列は実効スペック(タイプ + 制限)。
// ノードごとのスペックがある問題では1号機から順に全部並べる。
func TestRenderProblemsNodesAndTypeColumns(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	nodes := map[string]string{}
	types := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 4 {
			nodes[fields[0]] = fields[2]
			types[fields[0]] = fields[3]
		}
	}
	wantTypes := map[string]string{
		"isucon10-final":   "c5.large(mem=1G),c5.large(mem=2G),c5.xlarge(mem=1G)",
		"isucon10-qualify": "c5.large(1vCPU,mem=2G)",
		"isucon11-final":   "c5.large(mem=2G)",
		"isucon13":         "c5.large",
	}
	for name, want := range wantTypes {
		if got := types[name]; got != want {
			t.Errorf("%s type = %q, want %q", name, got, want)
		}
	}
	if got := nodes["isucon12-final"]; got != "5" {
		t.Errorf("isucon12-final nodes = %q, want 5", got)
	}
	if got := nodes["isucon13"]; got != "3" {
		t.Errorf("isucon13 nodes = %q, want 3", got)
	}
}

// BENCH CMD 列は `isuenv bench` が使えるかを示す。カタログにベンチの起動方法が
// 埋まっている問題だけ yes になり、それ以外は NOTES のリンクを読む必要がある。
func TestRenderProblemsBenchCmdColumn(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	benchCmd := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 6 {
			benchCmd[fields[0]] = fields[5]
		}
	}
	for _, name := range []string{"isucon14", "private-isu"} {
		if got := benchCmd[name]; got != "yes" {
			t.Errorf("%s bench cmd = %q, want yes", name, got)
		}
	}
	if got := benchCmd["isucon13"]; got != "-" {
		t.Errorf("isucon13 has no bench command yet, want a dash: got %q", got)
	}
}

// カタログが既定で起動するタイプは全て単価表に載っていること。載っていないタイプが
// 1台でも混じると `isuenv list` の EST COST が "-" になり、課金の目安が見えなくなる。
func TestCatalogInstanceTypesHavePrices(t *testing.T) {
	for _, p := range catalog.List() {
		types := []string{p.InstanceType, p.BenchInstanceType}
		for _, s := range p.NodeSpecs {
			types = append(types, s.InstanceType)
		}
		for _, typ := range types {
			if typ == "" {
				continue
			}
			if _, ok := engine.HourlyUSD(typ); !ok {
				t.Errorf("problem %s uses %s, which has no price in engine/cost.go", p.Name, typ)
			}
		}
	}
}
