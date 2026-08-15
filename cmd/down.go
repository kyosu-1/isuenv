package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down <problem>",
	Short: "Terminate a practice environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		return runDown(ctx, &engine.Engine{EC2: client}, args[0], cmd.OutOrStdout())
	},
}

func runDown(ctx context.Context, e *engine.Engine, name string, w io.Writer) error {
	ids, err := e.Down(ctx, name)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		fmt.Fprintf(w, "No running environment %q. Nothing to do.\n", name)
	} else {
		fmt.Fprintf(w, "Terminating %s: %v\n", name, ids)
	}
	// 対象なしの場合も再生成する。前回のdownで取りこぼした古いエントリをここで回収できる。
	return refreshSSHConfig(ctx, e, ids...)
}

func init() {
	rootCmd.AddCommand(downCmd)
}
