package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

func testProblem() catalog.Problem {
	return catalog.Problem{Name: "isucon13", AMIPattern: "isucon13-*", OwnerID: "839726181030", SSHUser: "ubuntu"}
}

func runningInstance(id, env, node, ip, privIP string) ec2types.Instance {
	return ec2types.Instance{
		InstanceId:       aws.String(id),
		State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		PublicIpAddress:  aws.String(ip),
		PrivateIpAddress: aws.String(privIP),
		InstanceType:     ec2types.InstanceTypeC5Large,
		LaunchTime:       aws.Time(time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)),
		Tags: []ec2types.Tag{
			{Key: aws.String(TagManaged), Value: aws.String("true")},
			{Key: aws.String(TagEnv), Value: aws.String(env)},
			{Key: aws.String(TagNode), Value: aws.String(node)},
			{Key: aws.String(TagExpires), Value: aws.String("2026-07-08T18:00:00Z")},
		},
	}
}

func TestUp_LaunchesNodesWithTagsAndTTL(t *testing.T) {
	PollInterval = time.Millisecond
	var runs []*ec2.RunInstancesInput
	m := &awsapi.Mock{
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			runs = append(runs, in)
			id := "i-" + strconv.Itoa(len(runs))
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String(id)}}}, nil
		},
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) == 0 {
				// Upの重複チェック: 既存環境なし
				return &ec2.DescribeInstancesOutput{}, nil
			}
			// waitRunning: 全ノードrunning
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
				runningInstance("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	nodes, err := e.Up(context.Background(), UpOptions{
		Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c5.large",
		TTL: 8 * time.Hour, KeyName: "isuenv",
		Net: Network{SubnetID: "subnet-1", SecurityGroupID: "sg-1"},
		Now: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Index != 1 || nodes[1].Index != 2 {
		t.Fatalf("expected 2 sorted nodes, got %+v", nodes)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 RunInstances calls, got %d", len(runs))
	}
	first := runs[0]
	if first.InstanceInitiatedShutdownBehavior != ec2types.ShutdownBehaviorTerminate {
		t.Error("shutdown behavior must be terminate (TTL self-destruction)")
	}
	ud, err := base64.StdEncoding.DecodeString(aws.ToString(first.UserData))
	if err != nil || !strings.Contains(string(ud), "shutdown -P +480") {
		t.Errorf("user data must contain TTL shutdown: %s (err %v)", ud, err)
	}
	tags := first.TagSpecifications[0].Tags
	if tagValue(tags, TagEnv) != "isucon13" || tagValue(tags, TagNode) != "1" {
		t.Errorf("env/node tags required: %+v", tags)
	}
	if tagValue(tags, TagExpires) != "2026-07-08T18:00:00Z" {
		t.Errorf("expires tag must be Now+TTL in RFC3339 UTC: %+v", tags)
	}
}

func TestUp_DuplicateEnvRejected(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-9", "isucon13", "1", "54.0.0.9", "10.100.0.19"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.Up(context.Background(), UpOptions{Problem: testProblem(), AMIID: "ami-123", Nodes: 1, InstanceType: "c5.large", TTL: time.Hour, KeyName: "isuenv", Now: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestUp_RollbackOnLaunchFailure(t *testing.T) {
	var terminated []string
	callCount := 0
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{}, nil
		},
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			callCount++
			if callCount == 2 {
				return nil, errors.New("InsufficientInstanceCapacity")
			}
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String("i-1")}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			terminated = in.InstanceIds
			return &ec2.TerminateInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.Up(context.Background(), UpOptions{Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c5.large", TTL: time.Hour, KeyName: "isuenv", Now: time.Now()})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(terminated) != 1 || terminated[0] != "i-1" {
		t.Errorf("launched instance must be rolled back: %v", terminated)
	}
}

func TestUp_EmptyRunInstancesResponseRolledBack(t *testing.T) {
	var terminated []string
	callCount := 0
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{}, nil
		},
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			callCount++
			if callCount == 2 {
				// AWSがエラーなしで空のInstancesを返すケース(現実には稀だが起きうる)
				return &ec2.RunInstancesOutput{}, nil
			}
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String("i-1")}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			terminated = in.InstanceIds
			return &ec2.TerminateInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.Up(context.Background(), UpOptions{Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c5.large", TTL: time.Hour, KeyName: "isuenv", Now: time.Now()})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(terminated) != 1 || terminated[0] != "i-1" {
		t.Errorf("launched instance must be rolled back: %v", terminated)
	}
}

func TestList_GroupsByEnv(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12"),
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
				runningInstance("i-3", "isucon14", "1", "54.0.0.3", "10.100.0.13"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	envs, err := e.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d: %+v", len(envs), envs)
	}
	if envs[0].Name != "isucon13" || envs[1].Name != "isucon14" {
		t.Errorf("envs must be sorted by name: %+v", envs)
	}
	if len(envs[0].Nodes) != 2 || envs[0].Nodes[0].Index != 1 || envs[0].Nodes[1].Index != 2 {
		t.Errorf("nodes must be sorted by index: %+v", envs[0].Nodes)
	}
	wantExpires := time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)
	if !envs[0].ExpiresAt.Equal(wantExpires) {
		t.Errorf("expires-at tag must be parsed: %v", envs[0].ExpiresAt)
	}
	if envs[0].InstanceType != "c5.large" {
		t.Errorf("instance type must be captured: %v", envs[0].InstanceType)
	}
}
