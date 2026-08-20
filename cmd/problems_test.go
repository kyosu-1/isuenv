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
	for _, want := range []string{"NAME", "TYPE", "BENCH TYPE", "isucon13", "isucon14", "ubuntu", "private-isu", "c7a.large", "c7a.xlarge"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

// BENCH TYPE 列は推奨値のある問題だけタイプを出し、無い問題は "-" で埋める
// (列が空のままだとタブ区切りが潰れて読めなくなるため)。
func TestRenderProblemsBenchTypeColumn(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	// NAME / SSH USER / TYPE / BENCH TYPE の4列目を問題ごとに拾う。
	benchTypes := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if fields := strings.Fields(line); len(fields) >= 4 {
			benchTypes[fields[0]] = fields[3]
		}
	}
	if got := benchTypes["private-isu"]; got != "c7a.xlarge" {
		t.Errorf("private-isu bench type = %q, want c7a.xlarge", got)
	}
	if got := benchTypes["isucon13"]; got != "-" {
		t.Errorf("isucon13 has no recommended bench type, want a dash: got %q", got)
	}
}
