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
	upNodeInstanceTypes []string
	upBench             bool
	upBenchInstanceType string
	upNoLimits          bool
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
		nodeSpecs, err := resolveNodeSpecs(upNodes, upInstanceType, upNodeInstanceTypes, upNoLimits, p)
		if err != nil {
			return err
		}
		benchSpec, err := resolveBenchSpec(upBench, upBenchInstanceType, upNoLimits, p)
		if err != nil {
			return err
		}
		fmt.Printf("Launching %d node(s) of %s (%s, TTL %s)...\n", upNodes, p.Name, describeNodeSpecs(nodeSpecs), upTTL)
		if benchSpec != nil {
			fmt.Printf("  plus 1 bench node (%s)\n", benchSpec.Label())
		}
		nodes, err := e.Up(ctx, engine.UpOptions{
			Problem: p, AMIID: ami.ID, Nodes: upNodes, NodeSpecs: nodeSpecs,
			Bench: benchSpec,
			TTL:   upTTL, KeyName: key, Net: net, Now: time.Now(),
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
		if note := memLimitNote(nodes); note != "" {
			fmt.Printf("\n%s\n", note)
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

// resolveNodeSpecs は競技ノード nodes 台それぞれのスペック(タイプとCPU・メモリの制限)を1号機から順に決める。
// 何も指定しなければカタログの値、つまり本番の公式スペックになる。カタログがノードごとの
// スペックを持つ問題(isucon10-final)では、それより多い台数の残りのノードは問題の既定スペックになる。
//
// タイプを明示した(--instance-type は全ノード、--node-instance-types は指定した番号の)ノードは、
// そのタイプで起動してCPUの制限を外す。CpuOptions の有効値はタイプごとに違い、カタログの値が
// 指定されたタイプで有効とは限らないため。メモリの制限(mem=)はタイプに依らず効くので残す。
// --node-instance-types で指定が足りない番号のノードは、カタログのスペックのまま。
// --no-limits は最後に全ノードの制限を外す(タイプだけで起動する抜け道)。
func resolveNodeSpecs(nodes int, flagType string, flagNodeTypes []string, noLimits bool, p catalog.Problem) ([]catalog.Spec, error) {
	if flagType != "" && len(flagNodeTypes) > 0 {
		return nil, fmt.Errorf("--instance-type and --node-instance-types cannot be used together")
	}
	if len(flagNodeTypes) > nodes {
		return nil, fmt.Errorf("--node-instance-types has %d types but --nodes is %d", len(flagNodeTypes), nodes)
	}
	for i, t := range flagNodeTypes {
		if strings.TrimSpace(t) == "" {
			return nil, fmt.Errorf("--node-instance-types has an empty type at position %d", i+1)
		}
	}
	specs := p.SpecsFor(nodes)
	for i := range specs {
		switch {
		case flagType != "":
			specs[i] = specs[i].WithInstanceType(flagType)
		case i < len(flagNodeTypes):
			specs[i] = specs[i].WithInstanceType(strings.TrimSpace(flagNodeTypes[i]))
		}
		if noLimits {
			specs[i] = specs[i].Unlimited()
		}
	}
	return specs, nil
}

// describeNodeSpecs は競技ノードのスペックを起動時の1行に載せる形にする。
// 全ノード同じなら1つだけ、違うなら1号機から順に全部並べる。
func describeNodeSpecs(specs []catalog.Spec) string {
	labels := make([]string, 0, len(specs))
	for _, s := range specs {
		labels = append(labels, s.Label())
	}
	if !mixed(labels) {
		if len(labels) == 0 {
			return ""
		}
		return labels[0]
	}
	return strings.Join(labels, ",")
}

func mixed(values []string) bool {
	for _, v := range values {
		if v != values[0] {
			return true
		}
	}
	return false
}

// resolveBenchSpec はベンチマーカー専用ノードのスペックを決める。nil はベンチノードなし。
// --bench-instance-type の明示指定があればそれを使う(--bench を付けなくてもベンチノードが追加される)。
// このときカタログのCPU制限は外し、メモリ制限は残す(競技ノードの --instance-type と同じ扱い)。
// --bench だけなら問題ごとのカタログ値を使うが、ベンチのスペックが非公開の問題にはカタログ値が無い。
// その場合に競技ノードと同じタイプへ黙って落とすと、ベンチ側が先に飽和するサイズで
// 起動してしまうので、エラーにして明示指定を求める。
func resolveBenchSpec(bench bool, flagValue string, noLimits bool, p catalog.Problem) (*catalog.Spec, error) {
	spec, ok := p.BenchSpec()
	switch {
	case flagValue != "":
		spec = spec.WithInstanceType(flagValue)
	case !bench:
		return nil, nil
	case !ok:
		return nil, fmt.Errorf("no recommended bench instance type for %q; pass --bench-instance-type explicitly", p.Name)
	}
	if noLimits {
		spec = spec.Unlimited()
	}
	return &spec, nil
}

// formatNodeLines は up の結果表示の行を組み立てる。
// ベンチノードがある構成、競技ノードのタイプが揃っていない構成、CPUやメモリを絞った構成でだけ
// タイプ(と制限)とロールの列を足す(どれがベンチか、どのノードがどのスペックかを判別できるようにするため)。
// 先頭の列はsshのホスト名そのものなので、ロール表示時は `(ssh ...)` の案内を省いて横幅を詰める。
func formatNodeLines(name string, nodes []engine.Node) []string {
	detailed := false
	labels := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Role == engine.RoleBench || n.LimitVCPUs > 0 || n.LimitMemGB > 0 {
			detailed = true
		}
		labels = append(labels, n.TypeLabel())
	}
	if !detailed && !mixed(labels) {
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
		fmt.Fprintf(tw, "  %s\tpublic %s\tprivate %s\t%s\t%s\n", engine.NodeName(name, n.Index, n.Role), n.PublicIP, n.PrivateIP, n.TypeLabel(), role)
	}
	tw.Flush()
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// memLimitNote はメモリを絞ったノードがあるときの案内。mem= はカーネル引数なので、
// 起動直後にuser-dataが1回再起動する。その間はsshが切れるので、知らないと障害に見える。
func memLimitNote(nodes []engine.Node) string {
	for _, n := range nodes {
		if n.LimitMemGB > 0 {
			return "Nodes with a mem= limit reboot once right after launch to apply it; ssh may be refused for a minute or two."
		}
	}
	return ""
}

func init() {
	upCmd.Flags().IntVar(&upNodes, "nodes", 1, "number of nodes to launch")
	upCmd.Flags().DurationVar(&upTTL, "ttl", 8*time.Hour, "auto-terminate after this duration")
	// 説明文のバックティックはcobraが引数プレースホルダ名として解釈するため使わない。
	upCmd.Flags().StringVar(&upInstanceType, "instance-type", "", "EC2 instance type for all nodes; drops the per-problem CPU limit (default: per-problem, see 'isuenv problems')")
	upCmd.Flags().StringSliceVar(&upNodeInstanceTypes, "node-instance-types", nil, "comma-separated EC2 instance types per node, from node 1 (default: per-problem)")
	upCmd.Flags().BoolVar(&upBench, "bench", false, "add one benchmarker node using the per-problem bench instance type")
	upCmd.Flags().StringVar(&upBenchInstanceType, "bench-instance-type", "", "EC2 instance type for the benchmarker node (implies --bench)")
	upCmd.Flags().BoolVar(&upNoLimits, "no-limits", false, "launch plain instance types without the per-problem CPU and memory limits")
	rootCmd.AddCommand(upCmd)
}
