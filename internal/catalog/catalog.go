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
	Notes        string `yaml:"notes"`
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
