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

// カタログのスペックは本番の公式スペックに合わせる。問題を足したり値を変えたりしたときに
// 気づけるよう、全問題の実効スペック(タイプ + CPU・メモリの制限)と本番の台数をここに固定する。
// 根拠は catalog.yaml の各問題のコメント。
func TestOfficialSpecs(t *testing.T) {
	type want struct {
		nodes int
		app   string // 1号機から本番の台数ぶんを並べたもの(全ノード同じなら1つ)
		bench string // 空はベンチのスペックが非公開
	}
	wants := map[string]want{
		"isucon9-qualify":  {3, "c5.large", "c5.4xlarge(12vCPU)"},
		"isucon9-final":    {3, "c5.large(mem=1G)", ""},
		"isucon10-qualify": {3, "c5.large(1vCPU,mem=2G)", "r5.large(1vCPU)"},
		"isucon10-final":   {3, "c5.large(mem=1G),c5.large(mem=2G),c5.xlarge(mem=1G)", "c5.2xlarge"},
		"isucon11-qualify": {3, "c5.large", "c4.xlarge"},
		"isucon11-final":   {3, "c5.large(mem=2G)", "c5.xlarge"},
		"isucon12-qualify": {3, "c5.large", "c5.xlarge"},
		"isucon12-final":   {5, "c5.large", "c5.xlarge"},
		"isucon13":         {3, "c5.large", "c5.2xlarge(mem=8G)"},
		"isucon14":         {3, "c5.large", "c5.2xlarge(mem=8G)"},
		"private-isu":      {1, "c7a.large", "c7a.xlarge"},
	}
	for _, p := range List() {
		w, ok := wants[p.Name]
		if !ok {
			t.Errorf("problem %s is missing from this test; decide its official spec explicitly", p.Name)
			continue
		}
		if p.OfficialNodes != w.nodes {
			t.Errorf("problem %s official nodes = %d, want %d", p.Name, p.OfficialNodes, w.nodes)
		}
		var labels []string
		for _, s := range p.SpecsFor(p.OfficialNodes) {
			if len(labels) == 0 || len(p.NodeSpecs) > 0 {
				labels = append(labels, s.Label())
			}
		}
		if got := strings.Join(labels, ","); got != w.app {
			t.Errorf("problem %s app spec = %q, want %q", p.Name, got, w.app)
		}
		bench := ""
		if s, ok := p.BenchSpec(); ok {
			bench = s.Label()
		}
		if bench != w.bench {
			t.Errorf("problem %s bench spec = %q, want %q", p.Name, bench, w.bench)
		}
	}
}

// カタログの cpu_options と mem_gb が、そのインスタンスタイプで実際に指定できる値かを確かめる。
// 無効な CpuOptions は RunInstances がエラーになり、タイプのメモリ以上の mem= は何も絞らない。
// 表は 2026-10-05 に ap-northeast-1 の `aws ec2 describe-instance-types` (VCpuInfo / MemoryInfo)で
// 確認した値。カタログに新しいタイプを足すときは、同じコマンドで確認してここにも足すこと。
func TestCatalogLimitsAreValidForInstanceType(t *testing.T) {
	type shape struct {
		validCores          []int32
		validThreadsPerCore []int32
		memMiB              int
	}
	shapes := map[string]shape{
		"c4.xlarge":  {[]int32{1, 2}, []int32{1, 2}, 7680},
		"c5.large":   {[]int32{1}, []int32{1, 2}, 4096},
		"c5.xlarge":  {[]int32{2}, []int32{1, 2}, 8192},
		"c5.2xlarge": {[]int32{2, 4}, []int32{1, 2}, 16384},
		"c5.4xlarge": {[]int32{2, 4, 6, 8}, []int32{1, 2}, 32768},
		"c7a.large":  {[]int32{1, 2}, []int32{1}, 4096},
		"c7a.xlarge": {[]int32{1, 2, 3, 4}, []int32{1}, 8192},
		"r5.large":   {[]int32{1}, []int32{1, 2}, 16384},
	}
	contains := func(list []int32, v int32) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	for _, p := range List() {
		specs := append([]Spec{p.DefaultSpec()}, p.NodeSpecs...)
		if s, ok := p.BenchSpec(); ok {
			specs = append(specs, s)
		}
		for _, s := range specs {
			// バースト系はCPUクレジットの残量でスコアが揺れるので使わない。
			if strings.HasPrefix(s.InstanceType, "t") {
				t.Errorf("problem %s uses burstable type %s", p.Name, s.InstanceType)
			}
			sh, ok := shapes[s.InstanceType]
			if !ok {
				t.Errorf("problem %s uses %s, which is missing from this test; check it with describe-instance-types", p.Name, s.InstanceType)
				continue
			}
			if c := s.CPUOptions; c != nil {
				if !contains(sh.validCores, c.CoreCount) || !contains(sh.validThreadsPerCore, c.ThreadsPerCore) {
					t.Errorf("problem %s: cpu_options %+v is not valid for %s (cores %v, threads per core %v)",
						p.Name, *c, s.InstanceType, sh.validCores, sh.validThreadsPerCore)
				}
			}
			if s.MemGB < 0 || s.MemGB*1024 >= sh.memMiB {
				t.Errorf("problem %s: mem_gb %d does not limit %s (%d MiB)", p.Name, s.MemGB, s.InstanceType, sh.memMiB)
			}
		}
	}
}

func TestSpecsFor(t *testing.T) {
	p := Problem{InstanceType: "c5.large", MemGB: 1, NodeSpecs: []Spec{
		{InstanceType: "c5.large", MemGB: 1},
		{InstanceType: "c5.large", MemGB: 2},
		{InstanceType: "c5.xlarge", MemGB: 1},
	}}
	label := func(specs []Spec) string {
		var labels []string
		for _, s := range specs {
			labels = append(labels, s.Label())
		}
		return strings.Join(labels, ",")
	}
	// 一覧より多い台数の残りは既定スペックになる。
	if got := label(p.SpecsFor(4)); got != "c5.large(mem=1G),c5.large(mem=2G),c5.xlarge(mem=1G),c5.large(mem=1G)" {
		t.Errorf("got %q", got)
	}
	// 少ない台数なら1号機から順に使う。
	if got := label(p.SpecsFor(2)); got != "c5.large(mem=1G),c5.large(mem=2G)" {
		t.Errorf("got %q", got)
	}
	uniform := Problem{InstanceType: "c5.large", CPUOptions: &CPUOptions{CoreCount: 1, ThreadsPerCore: 1}, MemGB: 2}
	if got := label(uniform.SpecsFor(2)); got != "c5.large(1vCPU,mem=2G),c5.large(1vCPU,mem=2G)" {
		t.Errorf("got %q", got)
	}
}

// node_specs で instance_type を省略したノードは、問題の既定タイプになる。
func TestListDefaultsNodeSpecInstanceType(t *testing.T) {
	for _, p := range List() {
		for i, s := range p.NodeSpecs {
			if s.InstanceType == "" {
				t.Errorf("problem %s node %d: instance type should have been defaulted by List()", p.Name, i+1)
			}
		}
	}
}

func TestSpecLabel(t *testing.T) {
	tests := []struct {
		spec Spec
		want string
	}{
		{Spec{InstanceType: "c5.large"}, "c5.large"},
		{Spec{InstanceType: "c5.large", MemGB: 2}, "c5.large(mem=2G)"},
		{Spec{InstanceType: "c5.4xlarge", CPUOptions: &CPUOptions{CoreCount: 6, ThreadsPerCore: 2}}, "c5.4xlarge(12vCPU)"},
		{Spec{InstanceType: "c5.large", CPUOptions: &CPUOptions{CoreCount: 1, ThreadsPerCore: 1}, MemGB: 2}, "c5.large(1vCPU,mem=2G)"},
	}
	for _, tt := range tests {
		if got := tt.spec.Label(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

// タイプを明示指定されたらCPUの制限は外し(そのタイプで有効とは限らない)、メモリの制限は残す。
func TestSpecOverrides(t *testing.T) {
	s := Spec{InstanceType: "c5.large", CPUOptions: &CPUOptions{CoreCount: 1, ThreadsPerCore: 1}, MemGB: 2}
	if got := s.WithInstanceType("c6i.large").Label(); got != "c6i.large(mem=2G)" {
		t.Errorf("WithInstanceType: got %q", got)
	}
	if got := s.Unlimited().Label(); got != "c5.large" {
		t.Errorf("Unlimited: got %q", got)
	}
	if !s.Limited() || s.Unlimited().Limited() {
		t.Error("Limited should report whether any limit is set")
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

// ベンチ用タイプは根拠がある問題にだけ設定する。根拠は上流が明記した推奨タイプか、
// 本番のベンチのスペック(CPU・メモリの制限と合わせて再現できるタイプにする。制限は TestOfficialSpecs)。
// 無根拠な既定値を配らないため、スペックが公開されていない isucon9-final は空のままであることを確かめる。
func TestBenchInstanceTypeOnlyWhereRecommended(t *testing.T) {
	want := map[string]string{
		"isucon9-qualify":  "c5.4xlarge",
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
