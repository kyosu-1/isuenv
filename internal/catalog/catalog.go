// Package catalog はバイナリ埋め込みのISUCON過去問カタログを提供する。
package catalog

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var raw []byte

// DefaultInstanceType は catalog.yaml で instance_type を省略した問題に使うインスタンスタイプ。
const DefaultInstanceType = "c5.large"

// CPUOptions は RunInstances の CpuOptions で絞るCPU構成。
// 有効な組み合わせはインスタンスタイプごとに決まっている(describe-instance-types の
// VCpuInfo.ValidCores / ValidThreadsPerCore)ので、カタログに書くときは確認結果をコメントに残すこと。
type CPUOptions struct {
	CoreCount      int32 `yaml:"core_count"`
	ThreadsPerCore int32 `yaml:"threads_per_core"`
}

// VCPUs はこの指定でOSから見えるvCPU数。
func (c CPUOptions) VCPUs() int {
	return int(c.CoreCount * c.ThreadsPerCore)
}

// Spec は1ノードの起動内容。インスタンスタイプだけでは本番のスペックに合わない問題のために、
// CPU(CpuOptions)とメモリ(カーネル引数 mem=)の制限を持てる。
type Spec struct {
	InstanceType string `yaml:"instance_type"`
	// CPUOptions が nil ならインスタンスタイプの既定のままにする。
	CPUOptions *CPUOptions `yaml:"cpu_options"`
	// MemGB が正なら、カーネル引数 mem=<MemGB>G でOSから見えるメモリを絞る
	// (上流の provisioning が grub に mem=1G / mem=2G を書いているのと同じ方法)。0 は制限なし。
	MemGB int `yaml:"mem_gb"`
}

// Limited はCPUかメモリのどちらかを絞っているか。
func (s Spec) Limited() bool {
	return s.CPUOptions != nil || s.MemGB > 0
}

// WithInstanceType はインスタンスタイプを明示指定されたときのスペックを返す。
// CpuOptions の有効値はタイプごとに違い、カタログの値が指定されたタイプで有効とは限らないので外す。
// メモリ制限はカーネル引数でタイプに依らず効くので残す。
func (s Spec) WithInstanceType(instanceType string) Spec {
	return Spec{InstanceType: instanceType, MemGB: s.MemGB}
}

// Unlimited は制限を全て外した、インスタンスタイプだけのスペックを返す。
func (s Spec) Unlimited() Spec {
	return Spec{InstanceType: s.InstanceType}
}

// Label はスペックを1語で表す。制限が無ければタイプ名だけ、あれば `c5.large(1vCPU,mem=2G)` の形。
// 表の列やタグ由来の表示で同じ形に揃えるため、空白を含めない。
func (s Spec) Label() string {
	vcpus := 0
	if s.CPUOptions != nil {
		vcpus = s.CPUOptions.VCPUs()
	}
	return FormatLabel(s.InstanceType, vcpus, s.MemGB)
}

// FormatLabel は Label の本体。起動済みインスタンスはタグから vCPU 数とメモリ制限を読むので、
// Spec を経由せずに同じ形を作れるようにしている。vcpus / memGB は 0 なら制限なし。
func FormatLabel(instanceType string, vcpus, memGB int) string {
	var limits []string
	if vcpus > 0 {
		limits = append(limits, fmt.Sprintf("%dvCPU", vcpus))
	}
	if memGB > 0 {
		limits = append(limits, fmt.Sprintf("mem=%dG", memGB))
	}
	if len(limits) == 0 {
		return instanceType
	}
	return instanceType + "(" + strings.Join(limits, ",") + ")"
}

type Problem struct {
	Name       string `yaml:"name"`
	AMIPattern string `yaml:"ami_pattern"`
	OwnerID    string `yaml:"owner_id"`
	SSHUser    string `yaml:"ssh_user"`
	// OfficialNodes は本番の競技サーバーの台数。表示用で、`up --nodes` の既定値(1)は変えない。
	OfficialNodes int `yaml:"official_nodes"`
	// InstanceType / CPUOptions / MemGB は競技ノードの既定スペック。本番の公式スペックに合わせる
	// (根拠は catalog.yaml の各問題のコメント)。
	// InstanceType は省略時に List() が DefaultInstanceType で埋めるため、
	// List()/Lookup() の戻り値では常に非空。
	InstanceType string      `yaml:"instance_type"`
	CPUOptions   *CPUOptions `yaml:"cpu_options"`
	MemGB        int         `yaml:"mem_gb"`
	// NodeSpecs は競技ノードごとのスペック(1号機から順)。
	// 本番で競技ノードのスペックが揃っていなかった問題にだけ設定する。空なら全ノードが既定スペックになる。
	// 台数がこれより多いときの残りのノードも既定スペックになる。
	NodeSpecs []Spec `yaml:"node_specs"`
	// BenchInstanceType はベンチマーカー専用ノードのインスタンスタイプ。
	// 上流が推奨タイプを明記していればそれを、本番のベンチのスペックだけが公開されていれば
	// それを再現できるタイプを設定する(根拠は catalog.yaml の各問題のコメント)。
	// どちらも無い問題は空のままで、`up --bench` を使うには --bench-instance-type での
	// 明示指定が要る(勝手な推奨値を作らないため、既定値では埋めない)。
	BenchInstanceType string `yaml:"bench_instance_type"`
	// BenchCPUOptions / BenchMemGB はベンチノードの制限。意味は競技ノードの CPUOptions / MemGB と同じ。
	BenchCPUOptions *CPUOptions `yaml:"bench_cpu_options"`
	BenchMemGB      int         `yaml:"bench_mem_gb"`
	// Bench はベンチマーカーの起動方法。nil の問題は `isuenv bench` に未対応で、
	// NOTES のリンク先を読んで手で打つことになる。
	// 起動方法は問題ごとに全く違ううえ実機で確認しないと確定できないので、
	// 検証できた問題から順に埋めていく(推測で埋めない)。
	Bench *Bench `yaml:"bench"`
	Notes string `yaml:"notes"`
}

// DefaultSpec は競技ノードの既定スペック。
func (p Problem) DefaultSpec() Spec {
	return Spec{InstanceType: p.InstanceType, CPUOptions: p.CPUOptions, MemGB: p.MemGB}
}

// SpecsFor は競技ノード n 台それぞれのスペックを1号機から順に返す。
// NodeSpecs に無い番号のノードは既定スペックになる。
func (p Problem) SpecsFor(n int) []Spec {
	specs := make([]Spec, n)
	for i := range specs {
		if i < len(p.NodeSpecs) {
			specs[i] = p.NodeSpecs[i]
			continue
		}
		specs[i] = p.DefaultSpec()
	}
	return specs
}

// BenchSpec はベンチマーカー専用ノードのスペック。カタログに無い問題では ok=false。
func (p Problem) BenchSpec() (Spec, bool) {
	if p.BenchInstanceType == "" {
		return Spec{}, false
	}
	return Spec{InstanceType: p.BenchInstanceType, CPUOptions: p.BenchCPUOptions, MemGB: p.BenchMemGB}, true
}

// Bench はベンチマーカーの起動方法。
type Bench struct {
	// User はベンチを実行するOSユーザー。多くの問題は isucon ユーザーでないと
	// 必要なファイルが読めない。
	User string `yaml:"user"`
	// Workdir はベンチの実行ディレクトリ。相対パスの初期データを読む問題があるため必要。
	Workdir string `yaml:"workdir"`
	Command string `yaml:"command"`
	// Args はテンプレート。展開できる変数は BenchVars を参照。
	Args []string `yaml:"args"`
}

// BenchVars は Bench.Args のテンプレート変数。
// ベンチの引数に埋めるIPは構成(台数、ベンチ専用ノードの有無)によって変わり、
// これを手でコピーするのが練習環境での一番の手間なので、CLIが埋める。
type BenchVars struct {
	// TargetIP はベンチ対象ノードのprivate IP。
	TargetIP string
	// BenchIP はベンチを実行するノード自身のprivate IP。
	// isucon14 の payment サーバのように、ベンチ側がbindするアドレスを
	// 引数で渡す必要がある問題で使う。
	BenchIP string
	// AllIPs は全競技ノードのprivate IPをカンマ区切りにしたもの。
	AllIPs string
}

// RenderArgs は Args のテンプレートを展開する。
// 未知の変数はカタログのタイプミスなので、黙って空文字にせずエラーにする。
func (b Bench) RenderArgs(v BenchVars) ([]string, error) {
	out := make([]string, 0, len(b.Args))
	for i, raw := range b.Args {
		tmpl, err := template.New("arg").Option("missingkey=error").Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("bench args[%d] %q: %w", i, raw, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, v); err != nil {
			return nil, fmt.Errorf("bench args[%d] %q: %w", i, raw, err)
		}
		out = append(out, buf.String())
	}
	return out, nil
}

type catalogFile struct {
	Problems []Problem `yaml:"problems"`
}

func List() []Problem {
	var f catalogFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		panic(fmt.Sprintf("embedded catalog.yaml is broken: %v", err))
	}
	for i := range f.Problems {
		if f.Problems[i].InstanceType == "" {
			f.Problems[i].InstanceType = DefaultInstanceType
		}
		for j := range f.Problems[i].NodeSpecs {
			if f.Problems[i].NodeSpecs[j].InstanceType == "" {
				f.Problems[i].NodeSpecs[j].InstanceType = f.Problems[i].InstanceType
			}
		}
	}
	return f.Problems
}

func Lookup(name string) (Problem, error) {
	for _, p := range List() {
		if p.Name == name {
			return p, nil
		}
	}
	return Problem{}, fmt.Errorf("unknown problem %q (run `isuenv problems` to list available problems)", name)
}
