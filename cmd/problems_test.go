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
	for _, want := range []string{"NAME", "TYPE", "BENCH TYPE", "BENCH CMD", "isucon13", "isucon14", "ubuntu", "private-isu", "c7a.large", "c7a.xlarge"} {
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
	// NAME / SSH USER / TYPE / BENCH TYPE の4列目を問題ごとに拾う。
	benchTypes := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 4 {
			benchTypes[fields[0]] = fields[3]
		}
	}
	if got := benchTypes["private-isu"]; got != "c7a.xlarge" {
		t.Errorf("private-isu bench type = %q, want c7a.xlarge", got)
	}
	if got := benchTypes["isucon9-final"]; got != "-" {
		t.Errorf("isucon9-final has no recommended bench type, want a dash: got %q", got)
	}
}

// TYPE 列は、ノードごとの推奨値がある問題では1号機から順に全部並べる。
func TestRenderProblemsNodeTypesColumn(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	types := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 3 {
			types[fields[0]] = fields[2]
		}
	}
	if got := types["isucon10-final"]; got != "c5.large,c5.large,c5.xlarge" {
		t.Errorf("isucon10-final type = %q, want c5.large,c5.large,c5.xlarge", got)
	}
	if got := types["isucon13"]; got != "c5.large" {
		t.Errorf("isucon13 type = %q, want c5.large", got)
	}
}

// BENCH CMD 列は `isuenv bench` が使えるかを示す。カタログにベンチの起動方法が
// 埋まっている問題だけ yes になり、それ以外は NOTES のリンクを読む必要がある。
func TestRenderProblemsBenchCmdColumn(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	benchCmd := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 5 {
			benchCmd[fields[0]] = fields[4]
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
		for _, typ := range append([]string{p.InstanceType, p.BenchInstanceType}, p.NodeInstanceTypes...) {
			if typ == "" {
				continue
			}
			if _, ok := engine.HourlyUSD(typ); !ok {
				t.Errorf("problem %s uses %s, which has no price in engine/cost.go", p.Name, typ)
			}
		}
	}
}
