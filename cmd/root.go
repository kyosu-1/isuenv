package cmd

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/spf13/cobra"
)

const region = "ap-northeast-1"

var rootCmd = &cobra.Command{
	Use:          "isuenv",
	Short:        "ISUCON practice environments on AWS EC2",
	Long:         "isuenv creates and destroys ISUCON practice environments on AWS EC2 using public AMIs from matsuu/aws-isucon.",
	SilenceUsage: true,
}

// Execute はCtrl-C(SIGINT)やSIGTERMでキャンセルされるctxを配ってコマンドを実行する。
// これにより、up中のポーリングループなどを即座に打ち切って rollback を発火させられる。
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return rootCmd.ExecuteContext(ctx)
}

func newEC2Client(ctx context.Context) (*ec2.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}

func sshDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ssh"
	}
	return filepath.Join(home, ".ssh")
}

func pemPath() string {
	return filepath.Join(sshDir(), "isuenv.pem")
}
