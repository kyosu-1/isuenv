package cmd

import (
	"fmt"
	"io"
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
	fmt.Fprintln(tw, "NAME\tSSH USER\tTYPE\tBENCH TYPE\tBENCH CMD\tNOTES")
	for _, p := range catalog.List() {
		// 推奨値のない問題は "-"。`up --bench` を使うには --bench-instance-type が要ることを示す。
		bench := "-"
		if p.BenchInstanceType != "" {
			bench = p.BenchInstanceType
		}
		// BENCH CMD は `isuenv bench` でコマンドを出せるか。カタログにベンチの
		// 起動方法が埋まっている問題だけ yes になる。埋まっていない問題は
		// NOTES のリンク先を読んで手で打つことになる。
		benchCmd := "-"
		if p.Bench != nil {
			benchCmd = "yes"
		}
		// ノードごとの推奨値がある問題は、1号機から順に並べる(台数がそれより多いときの残りは既定タイプ)。
		instanceType := p.InstanceType
		if len(p.NodeInstanceTypes) > 0 {
			instanceType = strings.Join(p.NodeInstanceTypes, ",")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.SSHUser, instanceType, bench, benchCmd, p.Notes)
	}
	tw.Flush()
}
