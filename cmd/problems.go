package cmd

import (
	"fmt"
	"io"
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
	fmt.Fprintln(tw, "NAME\tSSH USER\tNOTES")
	for _, p := range catalog.List() {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.SSHUser, p.Notes)
	}
	tw.Flush()
}
