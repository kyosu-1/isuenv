// Package catalog はバイナリ埋め込みのISUCON過去問カタログを提供する。
package catalog

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var raw []byte

type Problem struct {
	Name         string `yaml:"name"`
	AMIPattern   string `yaml:"ami_pattern"`
	OwnerID      string `yaml:"owner_id"`
	SSHUser      string `yaml:"ssh_user"`
	DefaultNodes int    `yaml:"default_nodes"`
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
