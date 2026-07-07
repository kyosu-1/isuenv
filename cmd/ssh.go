package cmd

import (
	"os"
	"os/exec"
	"regexp"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/kyosu-1/isuenv/internal/myip"
	"github.com/spf13/cobra"
)

var nodeSuffix = regexp.MustCompile(`-\d+$`)

var sshCmd = &cobra.Command{
	Use:   "ssh <problem>[-N]",
	Short: "SSH into a practice environment node (node 1 by default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		sgID, err := e.FindManagedSecurityGroup(ctx)
		if err != nil {
			return err
		}
		if sgID != "" {
			ip, err := myip.Get(ctx)
			if err != nil {
				return err
			}
			if err := e.EnsureIngress(ctx, sgID, ip); err != nil {
				return err
			}
		}
		if err := refreshSSHConfig(ctx, e); err != nil {
			return err
		}
		alias := args[0]
		if !nodeSuffix.MatchString(alias) {
			alias += "-1"
		}
		ssh := exec.CommandContext(ctx, "ssh", alias)
		ssh.Stdin = os.Stdin
		ssh.Stdout = os.Stdout
		ssh.Stderr = os.Stderr
		return ssh.Run()
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
