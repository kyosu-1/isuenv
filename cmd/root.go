package cmd

import (
	"context"
	"os"
	"path/filepath"

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

func Execute() error {
	return rootCmd.Execute()
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
