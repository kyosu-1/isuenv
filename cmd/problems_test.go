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
	for _, want := range []string{"NAME", "isucon13", "isucon14", "ubuntu"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}
