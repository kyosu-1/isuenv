package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderProblems(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	out := buf.String()
	for _, want := range []string{"NAME", "TYPE", "isucon13", "isucon14", "ubuntu", "private-isu", "c7a.large"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}
