package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

// PollInterval は待機ポーリングの間隔。テストで短縮するためvarにしている。
var PollInterval = 5 * time.Second

const maxPolls = 60

type Node struct {
	Index     int
	ID        string
	PublicIP  string
	PrivateIP string
}

type UpOptions struct {
	Problem      catalog.Problem
	AMIID        string
	Nodes        int
	InstanceType string
	TTL          time.Duration
	KeyName      string
	Net          Network
	Now          time.Time
}

// Up は環境を起動し、全ノードがrunningかつパブリックIP付与済みになるまで待つ。
// 途中で失敗した場合は起動済みインスタンスをterminateしてから失敗を返す。
func (e *Engine) Up(ctx context.Context, opts UpOptions) ([]Node, error) {
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

	expires := opts.Now.Add(opts.TTL).UTC().Format(time.RFC3339)
	userData := base64.StdEncoding.EncodeToString([]byte(BuildUserData(opts.TTL)))

	var ids []string
	for i := 1; i <= opts.Nodes; i++ {
		out, err := e.EC2.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:                           aws.String(opts.AMIID),
			InstanceType:                      ec2types.InstanceType(opts.InstanceType),
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
					{Key: aws.String(TagNode), Value: aws.String(strconv.Itoa(i))},
					{Key: aws.String(TagExpires), Value: aws.String(expires)},
					{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("%s-%d", name, i))},
				},
			}},
		})
		if err != nil {
			e.rollback(ids)
			return nil, fmt.Errorf("launch node %d of %s: %w (launched instances were rolled back)", i, name, err)
		}
		if len(out.Instances) == 0 {
			// エラーなしで空のInstancesが返るケース: out.Instances[0]への添字アクセスを避ける
			e.rollback(ids)
			return nil, fmt.Errorf("launch node %d of %s: empty RunInstances response (launched instances were rolled back)", i, name)
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
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		var nodes []Node
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				if inst.State == nil || inst.State.Name != ec2types.InstanceStateNameRunning || inst.PublicIpAddress == nil {
					continue
				}
				index, _ := strconv.Atoi(tagValue(inst.Tags, TagNode))
				nodes = append(nodes, Node{
					Index:     index,
					ID:        aws.ToString(inst.InstanceId),
					PublicIP:  aws.ToString(inst.PublicIpAddress),
					PrivateIP: aws.ToString(inst.PrivateIpAddress),
				})
			}
		}
		if len(nodes) == len(ids) {
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].Index < nodes[j].Index })
			return nodes, nil
		}
		time.Sleep(PollInterval)
	}
	return nil, fmt.Errorf("instances did not become running within %s", time.Duration(maxPolls)*PollInterval)
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
	Name         string
	InstanceType string
	LaunchedAt   time.Time
	ExpiresAt    time.Time
	Nodes        []Node
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
				env = &Env{Name: name, InstanceType: string(inst.InstanceType)}
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
			env.Nodes = append(env.Nodes, Node{
				Index:     index,
				ID:        aws.ToString(inst.InstanceId),
				PublicIP:  aws.ToString(inst.PublicIpAddress),
				PrivateIP: aws.ToString(inst.PrivateIpAddress),
			})
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
