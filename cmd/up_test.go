package cmd

import (
	"strings"
	"testing"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
)

func TestResolveInstanceType(t *testing.T) {
	p := catalog.Problem{Name: "private-isu", InstanceType: "c7a.large"}

	if got := resolveInstanceType("", p); got != "c7a.large" {
		t.Errorf("unset flag should fall back to the problem default: got %q", got)
	}
	if got := resolveInstanceType("c5.large", p); got != "c5.large" {
		t.Errorf("explicit flag should win: got %q", got)
	}
}

func TestResolvedAMILine(t *testing.T) {
	got := resolvedAMILine(engine.AMI{ID: "ami-0fcf9e8e8675a9ee4", Name: "isucon14-20260818100152"})
	if want := "  -> ami-0fcf9e8e8675a9ee4 (isucon14-20260818100152)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// 名前を持たないAMIでも空の括弧を出さない。
	if got := resolvedAMILine(engine.AMI{ID: "ami-123"}); got != "  -> ami-123" {
		t.Errorf("unnamed AMI should be printed without parens: got %q", got)
	}
}

func TestResolveBenchInstanceType(t *testing.T) {
	privateISU := catalog.Problem{Name: "private-isu", InstanceType: "c7a.large", BenchInstanceType: "c7a.xlarge"}
	isucon13 := catalog.Problem{Name: "isucon13", InstanceType: "c5.large"}

	// 従来どおり(どちらのフラグも無し)ならベンチノードは作らない。
	if got, err := resolveBenchInstanceType(false, "", privateISU); got != "" || err != nil {
		t.Errorf("no flags should mean no bench node: got %q, %v", got, err)
	}
	if got, err := resolveBenchInstanceType(true, "", privateISU); got != "c7a.xlarge" || err != nil {
		t.Errorf("--bench should use the catalog value: got %q, %v", got, err)
	}
	// タイプの明示指定は --bench を兼ねる。
	if got, err := resolveBenchInstanceType(false, "c7a.2xlarge", privateISU); got != "c7a.2xlarge" || err != nil {
		t.Errorf("explicit bench type implies --bench: got %q, %v", got, err)
	}
	if got, err := resolveBenchInstanceType(true, "c7a.2xlarge", privateISU); got != "c7a.2xlarge" || err != nil {
		t.Errorf("explicit bench type should win over the catalog value: got %q, %v", got, err)
	}
	// 推奨値のない問題で --bench だけ渡されたら、勝手に決めずに明示指定を促す。
	_, err := resolveBenchInstanceType(true, "", isucon13)
	if err == nil || !strings.Contains(err.Error(), "--bench-instance-type") {
		t.Errorf("problems without a recommended bench type must ask for --bench-instance-type: %v", err)
	}
	if got, err := resolveBenchInstanceType(false, "c5.xlarge", isucon13); got != "c5.xlarge" || err != nil {
		t.Errorf("explicit type should work for any problem: got %q, %v", got, err)
	}
}

func TestFormatNodeLines(t *testing.T) {
	// ベンチノードが無い構成の表示は従来のまま。
	appOnly := []engine.Node{
		{Index: 1, PublicIP: "1.2.3.4", PrivateIP: "10.100.0.1", InstanceType: "c5.large", Role: engine.RoleApp},
		{Index: 2, PublicIP: "5.6.7.8", PrivateIP: "10.100.0.2", InstanceType: "c5.large", Role: engine.RoleApp},
	}
	got := formatNodeLines("isucon13", appOnly)
	want := []string{
		"  isucon13-1  public 1.2.3.4  private 10.100.0.1  (ssh isucon13-1)",
		"  isucon13-2  public 5.6.7.8  private 10.100.0.2  (ssh isucon13-2)",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}

	// ベンチノードがあるときは、どれがベンチかをタイプとロールの列で示す。
	mixed := []engine.Node{
		{Index: 1, PublicIP: "1.2.3.4", PrivateIP: "10.100.0.1", InstanceType: "c7a.large", Role: engine.RoleApp},
		{Index: 2, PublicIP: "5.6.7.8", PrivateIP: "10.100.0.2", InstanceType: "c7a.xlarge", Role: engine.RoleBench},
	}
	lines := formatNodeLines("private-isu", mixed)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "c7a.large") || !strings.HasSuffix(lines[0], "app") {
		t.Errorf("app node line should show its type and role: %q", lines[0])
	}
	if !strings.Contains(lines[1], "c7a.xlarge") || !strings.HasSuffix(lines[1], "bench") {
		t.Errorf("bench node line should show its type and role: %q", lines[1])
	}
	if !strings.HasPrefix(lines[1], "  private-isu-2 ") {
		t.Errorf("first column must stay the ssh host name: %q", lines[1])
	}
}
