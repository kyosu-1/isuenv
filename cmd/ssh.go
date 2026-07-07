package cmd

import (
	"errors"
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
		if err := ssh.Run(); err != nil {
			// リモートコマンドの非ゼロ終了はisuenv自体のエラーではないので、
			// 「Error: exit status N」を出さずにそのままの終了コードで抜ける。
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
