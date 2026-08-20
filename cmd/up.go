package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/kyosu-1/isuenv/internal/myip"
	"github.com/spf13/cobra"
)

var (
	upNodes             int
	upTTL               time.Duration
	upInstanceType      string
	upBench             bool
	upBenchInstanceType string
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
		benchInstanceType, err := resolveBenchInstanceType(upBench, upBenchInstanceType, p)
		if err != nil {
			return err
		}
		fmt.Printf("Launching %d node(s) of %s (%s, TTL %s)...\n", upNodes, p.Name, instanceType, upTTL)
		if benchInstanceType != "" {
			fmt.Printf("  plus 1 bench node (%s)\n", benchInstanceType)
		}
		nodes, err := e.Up(ctx, engine.UpOptions{
			Problem: p, AMIID: ami.ID, Nodes: upNodes, InstanceType: instanceType,
			BenchInstanceType: benchInstanceType,
			TTL:               upTTL, KeyName: key, Net: net, Now: time.Now(),
		})
		if err != nil {
			return err
		}
		// ssh-config更新に失敗してもノードのIPは既に確保できているので、
		// まず結果を表示してからssh-config更新を試みる（失敗時は警告のみでnilを返す）。
		fmt.Printf("\n%s is ready. Auto-terminates in %s.\n\n", p.Name, upTTL)
		for _, line := range formatNodeLines(p.Name, nodes) {
			fmt.Println(line)
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

// resolveBenchInstanceType はベンチマーカー専用ノードのインスタンスタイプを決める。
// 空文字を返したらベンチノードは作らない(--bench も --bench-instance-type も無い従来の構成)。
// --bench-instance-type の明示指定は --bench を兼ねる。タイプを指定しておきながら
// ベンチノードが作られないのは意図と食い違うため。
func resolveBenchInstanceType(bench bool, flagValue string, p catalog.Problem) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if !bench {
		return "", nil
	}
	if p.BenchInstanceType == "" {
		return "", fmt.Errorf("problem %q has no recommended bench instance type; pass --bench-instance-type to choose one", p.Name)
	}
	return p.BenchInstanceType, nil
}

// formatNodeLines は up の結果表示の行を組み立てる。
// ベンチノードがある構成でだけタイプとロールの列を足す(どれがベンチかを判別できるようにするため)。
// 先頭の列はsshのホスト名そのものなので、ロール表示時は `(ssh ...)` の案内を省いて横幅を詰める。
func formatNodeLines(name string, nodes []engine.Node) []string {
	hasBench := false
	for _, n := range nodes {
		if n.Role == engine.RoleBench {
			hasBench = true
		}
	}
	if !hasBench {
		lines := make([]string, 0, len(nodes))
		for _, n := range nodes {
			lines = append(lines, fmt.Sprintf("  %s-%d  public %s  private %s  (ssh %s-%d)", name, n.Index, n.PublicIP, n.PrivateIP, name, n.Index))
		}
		return lines
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	for _, n := range nodes {
		role := n.Role
		if role == "" {
			role = engine.RoleApp
		}
		fmt.Fprintf(tw, "  %s-%d\tpublic %s\tprivate %s\t%s\t%s\n", name, n.Index, n.PublicIP, n.PrivateIP, n.InstanceType, role)
	}
	tw.Flush()
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

func init() {
	upCmd.Flags().IntVar(&upNodes, "nodes", 1, "number of nodes to launch")
	upCmd.Flags().DurationVar(&upTTL, "ttl", 8*time.Hour, "auto-terminate after this duration")
	// 説明文のバックティックはcobraが引数プレースホルダ名として解釈するため使わない。
	upCmd.Flags().StringVar(&upInstanceType, "instance-type", "", "EC2 instance type (default: per-problem, see 'isuenv problems')")
	upCmd.Flags().BoolVar(&upBench, "bench", false, "add one benchmarker node using the per-problem bench instance type")
	upCmd.Flags().StringVar(&upBenchInstanceType, "bench-instance-type", "", "EC2 instance type for the benchmarker node (implies --bench)")
	rootCmd.AddCommand(upCmd)
}
