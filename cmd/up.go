package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/kyosu-1/isuenv/internal/myip"
	"github.com/spf13/cobra"
)

var (
	upNodes        int
	upTTL          time.Duration
	upInstanceType string
)

var upCmd = &cobra.Command{
	Use:   "up <problem>",
	Short: "Create a practice environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		p, err := catalog.Lookup(args[0])
		if err != nil {
			return err
		}
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}

		fmt.Printf("Resolving AMI for %s...\n", p.Name)
		ami, err := e.ResolveAMI(ctx, p)
		if err != nil {
			return err
		}
		fmt.Println(resolvedAMILine(ami))
		fmt.Println("Ensuring network...")
		net, err := e.EnsureNetwork(ctx)
		if err != nil {
			return err
		}
		ip, err := myip.Get(ctx)
		if err != nil {
			return err
		}
		if err := e.EnsureIngress(ctx, net.SecurityGroupID, ip); err != nil {
			return err
		}
		key, err := e.EnsureKeyPair(ctx, pemPath())
		if err != nil {
			return err
		}
		instanceType := resolveInstanceType(upInstanceType, p)
		fmt.Printf("Launching %d node(s) of %s (%s, TTL %s)...\n", upNodes, p.Name, instanceType, upTTL)
		nodes, err := e.Up(ctx, engine.UpOptions{
			Problem: p, AMIID: ami.ID, Nodes: upNodes, InstanceType: instanceType,
			TTL: upTTL, KeyName: key, Net: net, Now: time.Now(),
		})
		if err != nil {
			return err
		}
		// ssh-config更新に失敗してもノードのIPは既に確保できているので、
		// まず結果を表示してからssh-config更新を試みる（失敗時は警告のみでnilを返す）。
		fmt.Printf("\n%s is ready. Auto-terminates in %s.\n\n", p.Name, upTTL)
		for _, n := range nodes {
			fmt.Printf("  %s-%d  public %s  private %s  (ssh %s-%d)\n", p.Name, n.Index, n.PublicIP, n.PrivateIP, p.Name, n.Index)
		}
		if err := refreshSSHConfig(ctx, e); err != nil {
			fmt.Fprintf(os.Stderr, "warning: ssh config update failed: %v\n", err)
		}
		if p.Notes != "" {
			fmt.Printf("\nNote: %s\n", p.Notes)
		}
		return nil
	},
}

// resolvedAMILine は解決したAMIを1行で表す。上流は同じ名前パターンのままAMIを差し替えるため、
// どのイメージで起動したかをここで示さないと手元から確認できない。
func resolvedAMILine(a engine.AMI) string {
	if a.Name == "" {
		return fmt.Sprintf("  -> %s", a.ID)
	}
	return fmt.Sprintf("  -> %s (%s)", a.ID, a.Name)
}

// resolveInstanceType は --instance-type の明示指定を優先し、未指定(空)なら問題ごとの推奨値を使う。
// 推奨値は問題によって異なる(private-isuはc7a.large)ため、フラグの既定値には持たせられない。
func resolveInstanceType(flagValue string, p catalog.Problem) string {
	if flagValue != "" {
		return flagValue
	}
	return p.InstanceType
}

func init() {
	upCmd.Flags().IntVar(&upNodes, "nodes", 1, "number of nodes to launch")
	upCmd.Flags().DurationVar(&upTTL, "ttl", 8*time.Hour, "auto-terminate after this duration")
	// 説明文のバックティックはcobraが引数プレースホルダ名として解釈するため使わない。
	upCmd.Flags().StringVar(&upInstanceType, "instance-type", "", "EC2 instance type (default: per-problem, see 'isuenv problems')")
	rootCmd.AddCommand(upCmd)
}
