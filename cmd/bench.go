package cmd

import (
	"fmt"
	"strings"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

// bench は実行せず、そのまま貼れるコマンドを出力する。
// 実行まで奪わないのは、ISUCONの参加者がベンチの前後(デプロイ・ログ退避・集計)を
// 自前のMakefileやスクリプトで回すのが定番だから。出力するだけなら
// `$(isuenv bench isucon14)` でも Makefile でも tmux でも好きに組み込める。
var benchCmd = &cobra.Command{
	Use:   "bench <problem>",
	Short: "Print the command to run the benchmarker for a running environment",
	Long: "Print a ready-to-paste ssh command that runs the benchmarker against a running environment.\n" +
		"It does not run anything; the private IPs that the benchmarker needs are filled in for you.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		name := args[0]
		bench, err := benchOrError(name)
		if err != nil {
			return err
		}
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		envs, err := e.List(ctx)
		if err != nil {
			return err
		}
		var env engine.Env
		for _, v := range envs {
			if v.Name == name {
				env = v
			}
		}
		if env.Name == "" {
			return fmt.Errorf("environment %q is not running; run `isuenv up %s` first", name, name)
		}
		host, vars, err := benchTarget(env)
		if err != nil {
			return err
		}
		line, err := benchCommandLine(host, bench, vars)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), line)
		return nil
	},
}

// benchOrError は問題のベンチ起動方法を返す。起動方法は問題ごとに全く違ううえ
// 実機で確認しないと確定できないので、検証できた問題から順に埋めている。
// 未対応の問題は推測で動かそうとせず、手順の在り処を示して止まる。
func benchOrError(name string) (catalog.Bench, error) {
	p, err := catalog.Lookup(name)
	if err != nil {
		return catalog.Bench{}, err
	}
	if p.Bench == nil {
		return catalog.Bench{}, fmt.Errorf(
			"problem %q has no benchmarker command in the catalog yet; see the NOTES link in `isuenv problems` and run it over ssh", name)
	}
	return *p.Bench, nil
}

// benchTarget はベンチを打つノードと、引数に埋める変数を決める。
// ベンチ専用ノードがあればそこで打つ(競技ノードで回すと負荷生成がアプリのCPUを
// 食ってスコアが正しく測れない)。無ければ1号機で、対象も自分自身になる。
// 返すのはそのノードのsshホスト名。
func benchTarget(env engine.Env) (string, catalog.BenchVars, error) {
	var appIPs []string
	var benchNode *engine.Node
	var firstApp *engine.Node
	for i := range env.Nodes {
		n := &env.Nodes[i]
		if n.Role == engine.RoleBench {
			benchNode = n
			continue
		}
		// isuenv:role タグを持たない古いインスタンスは競技ノードとして扱う。
		appIPs = append(appIPs, n.PrivateIP)
		if firstApp == nil {
			firstApp = n
		}
	}
	if firstApp == nil {
		return "", catalog.BenchVars{}, fmt.Errorf("environment %q has no application node to benchmark", env.Name)
	}
	runner := benchNode
	if runner == nil {
		runner = firstApp
	}
	return engine.NodeName(env.Name, runner.Index, runner.Role), catalog.BenchVars{
		TargetIP: firstApp.PrivateIP,
		BenchIP:  runner.PrivateIP,
		AllIPs:   strings.Join(appIPs, ","),
	}, nil
}

// benchCommandLine は貼ればそのまま動く ssh 一行を組み立てる。
func benchCommandLine(host string, b catalog.Bench, vars catalog.BenchVars) (string, error) {
	args, err := b.RenderArgs(vars)
	if err != nil {
		return "", err
	}
	var parts []string
	if b.Workdir != "" {
		parts = append(parts, "cd "+b.Workdir+" &&")
	}
	if b.User != "" {
		parts = append(parts, "sudo -u "+b.User)
	}
	parts = append(parts, b.Command)
	parts = append(parts, args...)
	remote := strings.Join(parts, " ")
	return fmt.Sprintf("ssh %s %s", host, shellQuote(remote)), nil
}

// shellQuote はシングルクォートで包む。出力はそのまま貼られる前提なので、
// 引数にシングルクォートが混じってもシェルとして壊れないようにする。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func init() {
	rootCmd.AddCommand(benchCmd)
}
