package cmd

import (
	"testing"

	"github.com/kyosu-1/isuenv/internal/catalog"
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
