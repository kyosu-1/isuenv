package cmd

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/spf13/cobra"
)

var problemsCmd = &cobra.Command{
	Use:   "problems",
	Short: "List available ISUCON problems",
	RunE: func(cmd *cobra.Command, args []string) error {
		renderProblems(cmd.OutOrStdout())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(problemsCmd)
}

func renderProblems(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSSH USER\tNODES\tTYPE\tBENCH TYPE\tBENCH CMD\tNOTES")
	for _, p := range catalog.List() {
		// NODES は本番の競技サーバーの台数(`up --nodes` の既定値ではない)。
		officialNodes := "-"
		if p.OfficialNodes > 0 {
			officialNodes = strconv.Itoa(p.OfficialNodes)
		}
		// カタログ値のない問題は "-"。`up --bench` を使うには --bench-instance-type が要ることを示す。
		bench := "-"
		if spec, ok := p.BenchSpec(); ok {
			bench = spec.Label()
		}
		// BENCH CMD は `isuenv bench` でコマンドを出せるか。カタログにベンチの
		// 起動方法が埋まっている問題だけ yes になる。埋まっていない問題は
		// NOTES のリンク先を読んで手で打つことになる。
		benchCmd := "-"
		if p.Bench != nil {
			benchCmd = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.SSHUser, officialNodes, typeColumn(p), bench, benchCmd, p.Notes)
	}
	tw.Flush()
}

// typeColumn は競技ノードの実効スペックを表す。タイプだけでは本番のスペックにならない問題は
// `c5.large(1vCPU,mem=2G)` のように制限を添える。
// ノードごとのスペックがある問題は、1号機から順に並べる(台数がそれより多いときの残りは既定スペック)。
func typeColumn(p catalog.Problem) string {
	if len(p.NodeSpecs) == 0 {
		return p.DefaultSpec().Label()
	}
	labels := make([]string, 0, len(p.NodeSpecs))
	for _, s := range p.NodeSpecs {
		labels = append(labels, s.Label())
	}
	return strings.Join(labels, ",")
}
