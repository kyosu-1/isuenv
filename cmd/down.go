package cmd

import (
	"fmt"

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
		e := &engine.Engine{EC2: client}
		ids, err := e.Down(ctx, args[0])
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			fmt.Printf("No running environment %q. Nothing to do.\n", args[0])
			return nil
		}
		fmt.Printf("Terminating %s: %v\n", args[0], ids)
		return refreshSSHConfig(ctx, e)
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
