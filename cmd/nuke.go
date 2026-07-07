package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var nukeCmd = &cobra.Command{
	Use:   "nuke",
	Short: "Delete ALL isuenv-managed resources (instances, key pair, VPC)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("This deletes ALL isuenv resources on AWS. Type 'yes' to continue: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(answer) != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		if err := e.Nuke(ctx); err != nil {
			return err
		}
		fmt.Println("All isuenv resources deleted.")
		return refreshSSHConfig(ctx, e)
	},
}

func init() {
	rootCmd.AddCommand(nukeCmd)
}
