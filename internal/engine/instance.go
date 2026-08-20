package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

// PollInterval は待機ポーリングの間隔。テストで短縮するためvarにしている。
var PollInterval = 5 * time.Second

const maxPolls = 60

type Node struct {
	Index        int
	ID           string
	PublicIP     string
	PrivateIP    string
	InstanceType string
	// Role は RoleApp か RoleBench。isuenv:role タグを持たない古いインスタンスでは空になる。
	Role string
}

type UpOptions struct {
	Problem      catalog.Problem
	AMIID        string
	Nodes        int
	InstanceType string
	// BenchInstanceType が非空なら、競技ノードの次の番号でベンチマーカー用ノードを1台追加する。
	// 空ならベンチノードは作らない。
	BenchInstanceType string
	TTL               time.Duration
	KeyName           string
	Net               Network
	Now               time.Time
}

// launch は1インスタンスの起動内容。RunInstancesの引数が(番号, タイプ, ロール)でしか
// 変わらないので、起動ループの中で分岐させずに先に組み立てておく。
type launch struct {
	index        int
	instanceType string
	role         string
}

// buildLaunches は起動するインスタンスの一覧を組み立てる。
// ベンチノードの番号を競技ノードの次にするのは、sshのホスト名を <問題名>-<番号> のまま保つため。
func buildLaunches(opts UpOptions) []launch {
	launches := make([]launch, 0, opts.Nodes+1)
	for i := 1; i <= opts.Nodes; i++ {
		launches = append(launches, launch{index: i, instanceType: opts.InstanceType, role: RoleApp})
	}
	if opts.BenchInstanceType != "" {
		launches = append(launches, launch{index: opts.Nodes + 1, instanceType: opts.BenchInstanceType, role: RoleBench})
	}
	return launches
}

// Up は環境を起動し、全ノードがrunningかつパブリックIP付与済みになるまで待つ。
// 途中で失敗した場合は起動済みインスタンスをterminateしてから失敗を返す。
func (e *Engine) Up(ctx context.Context, opts UpOptions) ([]Node, error) {
	if opts.Nodes < 1 {
		return nil, fmt.Errorf("nodes must be >= 1, got %d", opts.Nodes)
	}
	name := opts.Problem.Name
	existing, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			{Name: aws.String("tag:" + TagEnv), Values: []string{name}},
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("check existing env: %w", err)
	}
	for _, r := range existing.Reservations {
		if len(r.Instances) > 0 {
			return nil, fmt.Errorf("environment %q already exists; run `isuenv down %s` first", name, name)
		}
	}

	expiresAt := opts.Now.Add(opts.TTL)
	expires := expiresAt.UTC().Format(time.RFC3339)
	userData := base64.StdEncoding.EncodeToString([]byte(BuildUserData(expiresAt)))

	var ids []string
	for _, l := range buildLaunches(opts) {
		out, err := e.EC2.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:                           aws.String(opts.AMIID),
			InstanceType:                      ec2types.InstanceType(l.instanceType),
			MinCount:                          aws.Int32(1),
			MaxCount:                          aws.Int32(1),
			KeyName:                           aws.String(opts.KeyName),
			SubnetId:                          aws.String(opts.Net.SubnetID),
			SecurityGroupIds:                  []string{opts.Net.SecurityGroupID},
			UserData:                          aws.String(userData),
			InstanceInitiatedShutdownBehavior: ec2types.ShutdownBehaviorTerminate,
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String(TagManaged), Value: aws.String("true")},
					{Key: aws.String(TagEnv), Value: aws.String(name)},
					{Key: aws.String(TagNode), Value: aws.String(strconv.Itoa(l.index))},
					{Key: aws.String(TagExpires), Value: aws.String(expires)},
					{Key: aws.String(TagRole), Value: aws.String(l.role)},
					{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("%s-%d", name, l.index))},
				},
			}},
		})
		if err != nil {
			e.rollback(ids)
			return nil, fmt.Errorf("launch node %d of %s: %w (launched instances were rolled back)", l.index, name, err)
		}
		if len(out.Instances) == 0 {
			// エラーなしで空のInstancesが返るケース: out.Instances[0]への添字アクセスを避ける
			e.rollback(ids)
			return nil, fmt.Errorf("launch node %d of %s: empty RunInstances response (launched instances were rolled back)", l.index, name)
		}
		ids = append(ids, aws.ToString(out.Instances[0].InstanceId))
	}

	nodes, err := e.waitRunning(ctx, ids)
	if err != nil {
		e.rollback(ids)
		return nil, fmt.Errorf("wait for %s: %w (instances were rolled back)", name, err)
	}
	return nodes, nil
}

func (e *Engine) waitRunning(ctx context.Context, ids []string) ([]Node, error) {
	for attempt := 0; attempt < maxPolls; attempt++ {
		out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
		if err != nil {
			// RunInstances直後はDescribeInstancesが結果整合性によりInvalidInstanceID.NotFoundを
			// 返すことがある。これは失敗ではなくまだ反映されていないだけなので、リトライ対象として扱う。
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidInstanceID.NotFound" {
				if slErr := sleepOrDone(ctx); slErr != nil {
					return nil, slErr
				}
				continue
			}
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		var nodes []Node
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				if inst.State == nil || inst.State.Name != ec2types.InstanceStateNameRunning || inst.PublicIpAddress == nil {
					continue
				}
				index, _ := strconv.Atoi(tagValue(inst.Tags, TagNode))
				nodes = append(nodes, newNode(index, inst))
			}
		}
		if len(nodes) == len(ids) {
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].Index < nodes[j].Index })
			return nodes, nil
		}
		if err := sleepOrDone(ctx); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("instances did not become running within %s", time.Duration(maxPolls)*PollInterval)
}

// sleepOrDone はPollIntervalだけ待つか、ctxがキャンセルされたら即座にエラーを返す。
// Ctrl-C(SIGINT)でポーリングを打ち切れるようにするため、time.Sleepの代わりに使う。
func sleepOrDone(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(PollInterval):
		return nil
	}
}

// rollback はUp途中失敗時の後始末。失敗してもTTLで自己消滅するためエラーは握りつぶす。
// 呼び出し元のctxは失敗原因(タイムアウトやCtrl-Cによるキャンセル)である可能性があり、
// それをそのまま使うとTerminateInstancesが即座に無効化されてしまうため、専用の新しいctxを使う。
func (e *Engine) rollback(ids []string) {
	if len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = e.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids})
}

type Env struct {
	Name       string
	LaunchedAt time.Time
	ExpiresAt  time.Time
	Nodes      []Node
}

// InstanceTypeSummary は環境のインスタンスタイプを1行で表す。
// ベンチノードだけ別タイプという構成がありうるので、環境に単一のタイプを持たせるのではなく
// ノードごとのタイプから組み立てる。
func (env Env) InstanceTypeSummary() string {
	var app, bench []string
	for _, n := range env.Nodes {
		// isuenv:role タグを持たない古いインスタンスは競技ノードとして扱う。
		if n.Role == RoleBench {
			bench = appendUnique(bench, n.InstanceType)
			continue
		}
		app = appendUnique(app, n.InstanceType)
	}
	summary := strings.Join(app, ",")
	if len(bench) > 0 {
		summary += " +bench " + strings.Join(bench, ",")
	}
	return strings.TrimSpace(summary)
}

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

// newNode はDescribeInstancesの結果から1ノードを組み立てる。waitRunningとListで
// 同じ埋め方をする必要があるため関数に切り出している。
func newNode(index int, inst ec2types.Instance) Node {
	return Node{
		Index:        index,
		ID:           aws.ToString(inst.InstanceId),
		PublicIP:     aws.ToString(inst.PublicIpAddress),
		PrivateIP:    aws.ToString(inst.PrivateIpAddress),
		InstanceType: string(inst.InstanceType),
		Role:         tagValue(inst.Tags, TagRole),
	}
}

// List はisuenv管理下の稼働中環境を isuenv:env タグでグループ化して返す。
func (e *Engine) List(ctx context.Context) ([]Env, error) {
	out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instances: %w", err)
	}
	byName := map[string]*Env{}
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			name := tagValue(inst.Tags, TagEnv)
			if name == "" {
				continue
			}
			env, ok := byName[name]
			if !ok {
				env = &Env{Name: name}
				if v := tagValue(inst.Tags, TagExpires); v != "" {
					if ts, err := time.Parse(time.RFC3339, v); err == nil {
						env.ExpiresAt = ts
					}
				}
				byName[name] = env
			}
			launched := aws.ToTime(inst.LaunchTime)
			if env.LaunchedAt.IsZero() || launched.Before(env.LaunchedAt) {
				env.LaunchedAt = launched
			}
			index, _ := strconv.Atoi(tagValue(inst.Tags, TagNode))
			env.Nodes = append(env.Nodes, newNode(index, inst))
		}
	}
	envs := make([]Env, 0, len(byName))
	for _, env := range byName {
		sort.Slice(env.Nodes, func(i, j int) bool { return env.Nodes[i].Index < env.Nodes[j].Index })
		envs = append(envs, *env)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs, nil
}

// Down は環境のインスタンスをterminateし、対象のインスタンスIDを返す。対象なしは成功扱い。
func (e *Engine) Down(ctx context.Context, name string) ([]string, error) {
	out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			{Name: aws.String("tag:" + TagEnv), Values: []string{name}},
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instances for %s: %w", name, err)
	}
	var ids []string
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			ids = append(ids, aws.ToString(inst.InstanceId))
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := e.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids}); err != nil {
		return nil, fmt.Errorf("terminate %v: %w", ids, err)
	}
	return ids, nil
}

func (e *Engine) waitTerminated(ctx context.Context, ids []string) error {
	for attempt := 0; attempt < maxPolls; attempt++ {
		out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
		if err != nil {
			return fmt.Errorf("describe instances: %w", err)
		}
		done := true
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				if inst.State == nil || inst.State.Name != ec2types.InstanceStateNameTerminated {
					done = false
				}
			}
		}
		if done {
			return nil
		}
		if err := sleepOrDone(ctx); err != nil {
			return err
		}
	}
	return fmt.Errorf("instances did not terminate within %s", time.Duration(maxPolls)*PollInterval)
}
