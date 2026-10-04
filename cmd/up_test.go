package cmd

import (
	"strings"
	"testing"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
)

func labels(specs []catalog.Spec) string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Label())
	}
	return strings.Join(out, ",")
}

func TestResolveNodeSpecs(t *testing.T) {
	privateISU := catalog.Problem{Name: "private-isu", InstanceType: "c7a.large"}
	// CPUとメモリの両方を絞る問題。
	isucon10Qualify := catalog.Problem{
		Name: "isucon10-qualify", InstanceType: "c5.large",
		CPUOptions: &catalog.CPUOptions{CoreCount: 1, ThreadsPerCore: 1}, MemGB: 2,
	}
	// 本番で競技ノードのスペックが揃っていなかった問題。2号機だけメモリが多く、3号機だけコアが多い。
	isucon10Final := catalog.Problem{
		Name: "isucon10-final", InstanceType: "c5.large", MemGB: 1,
		NodeSpecs: []catalog.Spec{
			{InstanceType: "c5.large", MemGB: 1},
			{InstanceType: "c5.large", MemGB: 2},
			{InstanceType: "c5.xlarge", MemGB: 1},
		},
	}

	tests := []struct {
		name      string
		nodes     int
		flagType  string
		flagNodes []string
		noLimits  bool
		problem   catalog.Problem
		want      string
	}{
		{"unset flags fall back to the problem default", 2, "", nil, false, privateISU, "c7a.large,c7a.large"},
		{"explicit --instance-type wins", 2, "c5.large", nil, false, privateISU, "c5.large,c5.large"},
		{"catalog limits apply by default", 2, "", nil, false, isucon10Qualify, "c5.large(1vCPU,mem=2G),c5.large(1vCPU,mem=2G)"},
		// CpuOptions の有効値はタイプごとに違うので、タイプを明示されたらCPUの制限は外す。メモリの制限は残す。
		{"--instance-type drops the CPU limit and keeps the memory limit", 2, "c6i.large", nil, false, isucon10Qualify, "c6i.large(mem=2G),c6i.large(mem=2G)"},
		// カタログと同じタイプを明示しても、明示した以上はCPUの制限を外す(判定を型名に依存させない)。
		{"--instance-type with the catalog type still drops the CPU limit", 1, "c5.large", nil, false, isucon10Qualify, "c5.large(mem=2G)"},
		{"--no-limits launches the plain catalog type", 2, "", nil, true, isucon10Qualify, "c5.large,c5.large"},
		{"--no-limits with --instance-type launches the plain given type", 2, "c6i.large", nil, true, isucon10Qualify, "c6i.large,c6i.large"},
		{"per-node catalog specs are used in node order", 3, "", nil, false, isucon10Final, "c5.large(mem=1G),c5.large(mem=2G),c5.xlarge(mem=1G)"},
		{"fewer nodes take the first catalog specs", 1, "", nil, false, isucon10Final, "c5.large(mem=1G)"},
		{"nodes beyond the catalog list use the default spec", 4, "", nil, false, isucon10Final, "c5.large(mem=1G),c5.large(mem=2G),c5.xlarge(mem=1G),c5.large(mem=1G)"},
		{"--no-limits keeps per-node types", 3, "", nil, true, isucon10Final, "c5.large,c5.large,c5.xlarge"},
		// 全ノードを同じタイプにする指定なので、ノードごとのタイプより優先する。メモリの制限はノードごとのまま。
		{"--instance-type overrides per-node types", 3, "c7a.large", nil, false, isucon10Final, "c7a.large(mem=1G),c7a.large(mem=2G),c7a.large(mem=1G)"},
		{"--node-instance-types wins over the catalog", 3, "", []string{"c5.xlarge", "c5.large", "c5.large"}, false, isucon10Final, "c5.xlarge(mem=1G),c5.large(mem=2G),c5.large(mem=1G)"},
		// 指定が足りない番号のノードはカタログのスペックのまま。
		{"missing --node-instance-types entries keep the catalog spec", 3, "", []string{"c5.xlarge"}, false, isucon10Final, "c5.xlarge(mem=1G),c5.large(mem=2G),c5.xlarge(mem=1G)"},
		{"--node-instance-types drops the CPU limit only for the given nodes", 2, "", []string{"c6i.large"}, false, isucon10Qualify, "c6i.large(mem=2G),c5.large(1vCPU,mem=2G)"},
	}
	for _, tt := range tests {
		got, err := resolveNodeSpecs(tt.nodes, tt.flagType, tt.flagNodes, tt.noLimits, tt.problem)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tt.name, err)
			continue
		}
		if joined := labels(got); joined != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, joined, tt.want)
		}
	}

	// どちらを優先するか曖昧な組み合わせは、黙って片方を捨てずにエラーにする。
	if _, err := resolveNodeSpecs(3, "c5.large", []string{"c5.xlarge"}, false, isucon10Final); err == nil {
		t.Error("--instance-type with --node-instance-types should be rejected")
	}
	// 台数より多いタイプは起動されないノードの指定なので、打ち間違いとして扱う。
	if _, err := resolveNodeSpecs(2, "", []string{"c5.large", "c5.large", "c5.xlarge"}, false, isucon10Final); err == nil || !strings.Contains(err.Error(), "--nodes") {
		t.Errorf("more types than nodes should be rejected with a hint about --nodes: %v", err)
	}
	if _, err := resolveNodeSpecs(2, "", []string{"c5.large", ""}, false, isucon10Final); err == nil {
		t.Error("an empty type in --node-instance-types should be rejected")
	}
}

func TestDescribeNodeSpecs(t *testing.T) {
	if got := describeNodeSpecs([]catalog.Spec{{InstanceType: "c5.large", MemGB: 2}, {InstanceType: "c5.large", MemGB: 2}}); got != "c5.large(mem=2G)" {
		t.Errorf("uniform specs should be printed once: got %q", got)
	}
	// タイプが同じでも制限が違えば別のスペックとして並べる。
	if got := describeNodeSpecs([]catalog.Spec{{InstanceType: "c5.large", MemGB: 1}, {InstanceType: "c5.large", MemGB: 2}}); got != "c5.large(mem=1G),c5.large(mem=2G)" {
		t.Errorf("mixed specs should be listed in node order: got %q", got)
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

func TestResolveBenchSpec(t *testing.T) {
	privateISU := catalog.Problem{Name: "private-isu", InstanceType: "c7a.large", BenchInstanceType: "c7a.xlarge"}
	// ベンチのスペックが非公開でカタログ値を持たない問題。
	noBench := catalog.Problem{Name: "isucon9-final", InstanceType: "c5.large", MemGB: 1}
	// ベンチのCPUとメモリを絞る問題(実在のカタログには両方絞る問題は無いが、扱いを確かめるため)。
	limited := catalog.Problem{
		Name: "limited", InstanceType: "c5.large", BenchInstanceType: "c5.4xlarge",
		BenchCPUOptions: &catalog.CPUOptions{CoreCount: 6, ThreadsPerCore: 2}, BenchMemGB: 8,
	}
	label := func(s *catalog.Spec) string {
		if s == nil {
			return ""
		}
		return s.Label()
	}

	// 従来どおり(どちらのフラグも無し)ならベンチノードは作らない。
	if got, err := resolveBenchSpec(false, "", false, privateISU); got != nil || err != nil {
		t.Errorf("no flags should mean no bench node: got %v, %v", got, err)
	}
	if got, err := resolveBenchSpec(true, "", false, privateISU); label(got) != "c7a.xlarge" || err != nil {
		t.Errorf("--bench should use the catalog value: got %q, %v", label(got), err)
	}
	// タイプの明示指定は --bench を兼ねる。
	if got, err := resolveBenchSpec(false, "c7a.2xlarge", false, privateISU); label(got) != "c7a.2xlarge" || err != nil {
		t.Errorf("explicit bench type implies --bench: got %q, %v", label(got), err)
	}
	if got, err := resolveBenchSpec(true, "c7a.2xlarge", false, privateISU); label(got) != "c7a.2xlarge" || err != nil {
		t.Errorf("explicit bench type should win over the catalog value: got %q, %v", label(got), err)
	}
	// カタログ値のない問題で --bench だけ渡されたら、勝手に決めずに明示指定を促す。
	_, err := resolveBenchSpec(true, "", false, noBench)
	if err == nil || !strings.Contains(err.Error(), "--bench-instance-type") {
		t.Errorf("problems without a bench spec must ask for --bench-instance-type: %v", err)
	}
	// 明示指定ならどの問題でも通る。競技ノードのメモリ制限がベンチに漏れないこと。
	if got, err := resolveBenchSpec(false, "c5.xlarge", false, noBench); label(got) != "c5.xlarge" || err != nil {
		t.Errorf("explicit type should work for any problem: got %q, %v", label(got), err)
	}

	if got, err := resolveBenchSpec(true, "", false, limited); label(got) != "c5.4xlarge(12vCPU,mem=8G)" || err != nil {
		t.Errorf("--bench should apply the catalog limits: got %q, %v", label(got), err)
	}
	// タイプを明示されたらCPUの制限は外し、メモリの制限は残す(競技ノードと同じ扱い)。
	if got, err := resolveBenchSpec(true, "c7a.4xlarge", false, limited); label(got) != "c7a.4xlarge(mem=8G)" || err != nil {
		t.Errorf("explicit bench type should drop the CPU limit only: got %q, %v", label(got), err)
	}
	if got, err := resolveBenchSpec(true, "", true, limited); label(got) != "c5.4xlarge" || err != nil {
		t.Errorf("--no-limits should launch the plain bench type: got %q, %v", label(got), err)
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

	// 競技ノードのタイプが揃っていない構成では、ベンチノードが無くてもタイプを示す。
	uneven := []engine.Node{
		{Index: 1, PublicIP: "1.2.3.4", PrivateIP: "10.100.0.1", InstanceType: "c5.large", Role: engine.RoleApp},
		{Index: 2, PublicIP: "5.6.7.8", PrivateIP: "10.100.0.2", InstanceType: "c5.xlarge", Role: engine.RoleApp},
	}
	unevenLines := formatNodeLines("isucon10-final", uneven)
	if len(unevenLines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(unevenLines), unevenLines)
	}
	if !strings.Contains(unevenLines[0], "c5.large") || !strings.Contains(unevenLines[1], "c5.xlarge") {
		t.Errorf("nodes of different types should show their types: %q", unevenLines)
	}

	// CPUやメモリを絞った構成では、全ノード同じでも制限を示す。
	limited := []engine.Node{
		{Index: 1, PublicIP: "1.2.3.4", PrivateIP: "10.100.0.1", InstanceType: "c5.large", Role: engine.RoleApp, LimitVCPUs: 1, LimitMemGB: 2},
		{Index: 2, PublicIP: "5.6.7.8", PrivateIP: "10.100.0.2", InstanceType: "c5.large", Role: engine.RoleApp, LimitVCPUs: 1, LimitMemGB: 2},
	}
	for i, line := range formatNodeLines("isucon10-qualify", limited) {
		if !strings.Contains(line, "c5.large(1vCPU,mem=2G)") {
			t.Errorf("limited node line %d should show its limits: %q", i, line)
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
	if !strings.HasPrefix(lines[1], "  private-isu-bench ") {
		t.Errorf("first column must stay the ssh host name: %q", lines[1])
	}
}

// メモリを絞ったノードは起動直後に1回再起動するので、そのときだけ案内を出す。
func TestMemLimitNote(t *testing.T) {
	plain := []engine.Node{{Index: 1, InstanceType: "c5.large", LimitVCPUs: 1}}
	if got := memLimitNote(plain); got != "" {
		t.Errorf("nodes without a memory limit do not reboot, want no note: got %q", got)
	}
	limited := []engine.Node{{Index: 1, InstanceType: "c5.large"}, {Index: 2, InstanceType: "c5.large", LimitMemGB: 2}}
	if got := memLimitNote(limited); !strings.Contains(got, "reboot") {
		t.Errorf("a memory-limited node should produce a reboot note: got %q", got)
	}
}
