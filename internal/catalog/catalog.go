// Package catalog はバイナリ埋め込みのISUCON過去問カタログを提供する。
package catalog

import (
	_ "embed"
	"fmt"

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
	// 上流が明確な推奨値を出している問題にだけ設定する。空の問題で `up --bench` を使うには
	// --bench-instance-type での明示指定が要る(勝手な推奨値を作らないため、既定値では埋めない)。
	BenchInstanceType string `yaml:"bench_instance_type"`
	Notes             string `yaml:"notes"`
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
