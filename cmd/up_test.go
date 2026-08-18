package cmd

import (
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
