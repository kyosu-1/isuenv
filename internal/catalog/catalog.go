// Package catalog はバイナリ埋め込みのISUCON過去問カタログを提供する。
package catalog

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var raw []byte

// DefaultInstanceType は catalog.yaml で instance_type を省略した問題に使うインスタンスタイプ。
const DefaultInstanceType = "c5.large"

type Problem struct {
	Name       string `yaml:"name"`
	AMIPattern string `yaml:"ami_pattern"`
	OwnerID    string `yaml:"owner_id"`
	SSHUser    string `yaml:"ssh_user"`
	// InstanceType は問題ごとの推奨インスタンスタイプ。省略時は List() が DefaultInstanceType で埋めるため、
	// List()/Lookup() の戻り値では常に非空。
	InstanceType string `yaml:"instance_type"`
	// BenchInstanceType はベンチマーカー専用ノードの推奨インスタンスタイプ。
	// 上流が推奨タイプを明記していればそれを、本番のベンチのスペックだけが公開されていれば
	// そのvCPUとメモリを満たす最小のタイプを設定する(根拠は catalog.yaml の各問題のコメント)。
	// どちらも無い問題は空のままで、`up --bench` を使うには --bench-instance-type での
	// 明示指定が要る(勝手な推奨値を作らないため、既定値では埋めない)。
	BenchInstanceType string `yaml:"bench_instance_type"`
	// Bench はベンチマーカーの起動方法。nil の問題は `isuenv bench` に未対応で、
	// NOTES のリンク先を読んで手で打つことになる。
	// 起動方法は問題ごとに全く違ううえ実機で確認しないと確定できないので、
	// 検証できた問題から順に埋めていく(推測で埋めない)。
	Bench *Bench `yaml:"bench"`
	Notes string `yaml:"notes"`
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
