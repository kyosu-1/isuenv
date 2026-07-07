package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running practice environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		envs, err := e.List(ctx)
		if err != nil {
			return err
		}
		if len(envs) == 0 {
			fmt.Println("No running environments.")
			return nil
		}
		now := time.Now()
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ENV\tNODES\tTYPE\tUPTIME\tEST COST\tTTL LEFT\tPUBLIC IPS")
		for _, env := range envs {
			cost := "-"
			if h, ok := engine.HourlyUSD(env.InstanceType); ok {
				cost = fmt.Sprintf("$%.2f", engine.Estimate(env.LaunchedAt, now, h, len(env.Nodes)))
			}
			ttlLeft := "-"
			if !env.ExpiresAt.IsZero() {
				if remaining := time.Until(env.ExpiresAt); remaining < 0 {
					ttlLeft = "expired"
				} else {
					ttlLeft = remaining.Round(time.Minute).String()
				}
			}
			var ips []string
			for _, n := range env.Nodes {
				ips = append(ips, n.PublicIP)
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				env.Name, len(env.Nodes), env.InstanceType,
				now.Sub(env.LaunchedAt).Round(time.Minute), cost, ttlLeft, strings.Join(ips, ", "))
		}
		return tw.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
