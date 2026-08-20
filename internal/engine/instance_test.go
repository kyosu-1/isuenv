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
	"github.com/aws/smithy-go"
	"github.com/kyosu-1/isuenv/internal/awsapi"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

// notFoundError はRunInstances直後にDescribeInstancesが返しうる
// InvalidInstanceID.NotFound（結果整合性による一時的な404）を模したsmithy.APIError実装。
type notFoundError struct{}

func (notFoundError) Error() string                 { return "InvalidInstanceID.NotFound: not found" }
func (notFoundError) ErrorCode() string             { return "InvalidInstanceID.NotFound" }
func (notFoundError) ErrorMessage() string          { return "not found" }
func (notFoundError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

var _ smithy.APIError = notFoundError{}

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

// runningInstanceWithRole はロールとインスタンスタイプを明示したrunningインスタンス。
func runningInstanceWithRole(id, env, node, ip, privIP, instType, role string) ec2types.Instance {
	inst := runningInstance(id, env, node, ip, privIP)
	inst.InstanceType = ec2types.InstanceType(instType)
	inst.Tags = append(inst.Tags, ec2types.Tag{Key: aws.String(TagRole), Value: aws.String(role)})
	return inst
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
	// 2026-07-08T18:00:00Z (Now=10:00 + TTL=8h) のunixエポック秒。
	if err != nil || !strings.Contains(string(ud), "1783533600") {
		t.Errorf("user data must contain TTL expiry epoch: %s (err %v)", ud, err)
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
	// isuenv:role タグを持たない(この機能より前に起動した)インスタンスも競技ノードとして扱う。
	if envs[0].InstanceTypeSummary() != "c5.large" {
		t.Errorf("instance type must be captured: %v", envs[0].InstanceTypeSummary())
	}
}

func TestDown_TerminatesEnvInstances(t *testing.T) {
	var describeIn *ec2.DescribeInstancesInput
	var terminated []string
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			describeIn = in
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
				runningInstance("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12"),
			}}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			terminated = in.InstanceIds
			return &ec2.TerminateInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	ids, err := e.Down(context.Background(), "isucon13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || len(terminated) != 2 {
		t.Errorf("expected 2 instances terminated: ids=%v terminated=%v", ids, terminated)
	}
	found := false
	for _, f := range describeIn.Filters {
		if aws.ToString(f.Name) == "tag:"+TagEnv && f.Values[0] == "isucon13" {
			found = true
		}
	}
	if !found {
		t.Errorf("describe must filter by env tag: %+v", describeIn.Filters)
	}
}

func TestDown_NoInstancesIsNoop(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	ids, err := e.Down(context.Background(), "isucon13")
	if err != nil {
		t.Fatalf("down must be idempotent: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected no ids, got %v", ids)
	}
}

func TestUp_RejectsNodesBelowOne(t *testing.T) {
	e := &Engine{EC2: &awsapi.Mock{}}
	_, err := e.Up(context.Background(), UpOptions{Problem: testProblem(), AMIID: "ami-123", Nodes: 0, InstanceType: "c5.large", TTL: time.Hour, KeyName: "isuenv", Now: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "nodes must be >= 1") {
		t.Fatalf("expected nodes validation error, got %v", err)
	}
}

func TestUp_TreatsEventualConsistencyNotFoundAsNotReady(t *testing.T) {
	PollInterval = time.Millisecond
	describeCalls := 0
	m := &awsapi.Mock{
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String("i-1")}}}, nil
		},
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) == 0 {
				return &ec2.DescribeInstancesOutput{}, nil
			}
			describeCalls++
			if describeCalls == 1 {
				// RunInstances直後の結果整合性によるNotFound。失敗ではなくリトライすべき。
				return nil, notFoundError{}
			}
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	nodes, err := e.Up(context.Background(), UpOptions{
		Problem: testProblem(), AMIID: "ami-123", Nodes: 1, InstanceType: "c5.large",
		TTL: time.Hour, KeyName: "isuenv", Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("transient InvalidInstanceID.NotFound must not fail Up: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %+v", nodes)
	}
	if describeCalls < 2 {
		t.Fatalf("expected a retry after NotFound, got %d describe calls", describeCalls)
	}
}

// --bench 未指定なら従来どおり全ノードが同じタイプの競技ノードになる。
func TestUp_WithoutBenchLaunchesAppNodesOnly(t *testing.T) {
	PollInterval = time.Millisecond
	var runs []*ec2.RunInstancesInput
	m := &awsapi.Mock{
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			runs = append(runs, in)
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String("i-" + strconv.Itoa(len(runs)))}}}, nil
		},
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) == 0 {
				return &ec2.DescribeInstancesOutput{}, nil
			}
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstanceWithRole("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11", "c5.large", RoleApp),
				runningInstanceWithRole("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12", "c5.large", RoleApp),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	nodes, err := e.Up(context.Background(), UpOptions{
		Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c5.large",
		TTL: time.Hour, KeyName: "isuenv",
		Now: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected exactly 2 RunInstances calls, got %d", len(runs))
	}
	for i, run := range runs {
		if run.InstanceType != ec2types.InstanceType("c5.large") {
			t.Errorf("run %d: instance type = %q, want c5.large", i, run.InstanceType)
		}
		tags := run.TagSpecifications[0].Tags
		if got := tagValue(tags, TagRole); got != RoleApp {
			t.Errorf("run %d: role tag = %q, want %q", i, got, RoleApp)
		}
		if got := tagValue(tags, TagNode); got != strconv.Itoa(i+1) {
			t.Errorf("run %d: node tag = %q, want %d", i, got, i+1)
		}
	}
	for _, n := range nodes {
		if n.Role != RoleApp || n.InstanceType != "c5.large" {
			t.Errorf("node %d: got role %q type %q, want app/c5.large", n.Index, n.Role, n.InstanceType)
		}
	}
}

// --bench 相当(BenchInstanceType 指定)ならノード数+1台起動し、最後の1台だけタイプとロールが変わる。
func TestUp_BenchNodeUsesItsOwnTypeAndRole(t *testing.T) {
	PollInterval = time.Millisecond
	var runs []*ec2.RunInstancesInput
	m := &awsapi.Mock{
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			runs = append(runs, in)
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String("i-" + strconv.Itoa(len(runs)))}}}, nil
		},
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) == 0 {
				return &ec2.DescribeInstancesOutput{}, nil
			}
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstanceWithRole("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11", "c7a.large", RoleApp),
				runningInstanceWithRole("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12", "c7a.large", RoleApp),
				runningInstanceWithRole("i-3", "isucon13", "3", "54.0.0.3", "10.100.0.13", "c7a.xlarge", RoleBench),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	nodes, err := e.Up(context.Background(), UpOptions{
		Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c7a.large",
		BenchInstanceType: "c7a.xlarge", TTL: time.Hour, KeyName: "isuenv",
		Now: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected nodes+1 RunInstances calls, got %d", len(runs))
	}
	for i, run := range runs[:2] {
		if run.InstanceType != ec2types.InstanceType("c7a.large") {
			t.Errorf("app run %d: instance type = %q, want c7a.large", i, run.InstanceType)
		}
		if got := tagValue(run.TagSpecifications[0].Tags, TagRole); got != RoleApp {
			t.Errorf("app run %d: role tag = %q, want %q", i, got, RoleApp)
		}
	}
	benchRun := runs[2]
	if benchRun.InstanceType != ec2types.InstanceType("c7a.xlarge") {
		t.Errorf("bench instance type = %q, want c7a.xlarge", benchRun.InstanceType)
	}
	benchTags := benchRun.TagSpecifications[0].Tags
	if got := tagValue(benchTags, TagRole); got != RoleBench {
		t.Errorf("bench role tag = %q, want %q", got, RoleBench)
	}
	// ベンチノードの番号は競技ノードの次。sshのホスト名が <問題名>-<番号> のままであること。
	if got := tagValue(benchTags, TagNode); got != "3" {
		t.Errorf("bench node tag = %q, want 3", got)
	}
	if got := tagValue(benchTags, "Name"); got != "isucon13-3" {
		t.Errorf("bench Name tag = %q, want isucon13-3", got)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %+v", nodes)
	}
	if nodes[2].Role != RoleBench || nodes[2].InstanceType != "c7a.xlarge" {
		t.Errorf("last node must be the bench node: %+v", nodes[2])
	}
	for _, n := range nodes[:2] {
		if n.Role != RoleApp || n.InstanceType != "c7a.large" {
			t.Errorf("node %d must stay an app node: %+v", n.Index, n)
		}
	}
}

func TestBuildLaunches(t *testing.T) {
	withoutBench := buildLaunches(UpOptions{Nodes: 3, InstanceType: "c5.large"})
	if len(withoutBench) != 3 {
		t.Fatalf("expected 3 launches, got %+v", withoutBench)
	}
	withBench := buildLaunches(UpOptions{Nodes: 3, InstanceType: "c7a.large", BenchInstanceType: "c7a.2xlarge"})
	if len(withBench) != 4 {
		t.Fatalf("expected 4 launches, got %+v", withBench)
	}
	want := launch{index: 4, instanceType: "c7a.2xlarge", role: RoleBench}
	if withBench[3] != want {
		t.Errorf("bench launch = %+v, want %+v", withBench[3], want)
	}
}

func TestEnvInstanceTypeSummary(t *testing.T) {
	tests := []struct {
		name  string
		nodes []Node
		want  string
	}{
		{"single type", []Node{{InstanceType: "c5.large", Role: RoleApp}, {InstanceType: "c5.large", Role: RoleApp}}, "c5.large"},
		// isuenv:role タグが無い古いインスタンスでも従来どおりタイプだけを表示する。
		{"legacy without role tag", []Node{{InstanceType: "c5.large"}}, "c5.large"},
		{"with bench", []Node{
			{InstanceType: "c7a.large", Role: RoleApp},
			{InstanceType: "c7a.xlarge", Role: RoleBench},
		}, "c7a.large +bench c7a.xlarge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Env{Nodes: tt.nodes}).InstanceTypeSummary(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// listはインスタンスのタグとタイプからロールを復元する(CLIはローカルに状態を持たないため)。
func TestList_CapturesBenchRole(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstanceWithRole("i-1", "private-isu", "1", "54.0.0.1", "10.100.0.11", "c7a.large", RoleApp),
				runningInstanceWithRole("i-2", "private-isu", "2", "54.0.0.2", "10.100.0.12", "c7a.xlarge", RoleBench),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	envs, err := e.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 env, got %+v", envs)
	}
	if got := envs[0].InstanceTypeSummary(); got != "c7a.large +bench c7a.xlarge" {
		t.Errorf("summary = %q", got)
	}
	if envs[0].Nodes[1].Role != RoleBench {
		t.Errorf("bench role must be restored from the tag: %+v", envs[0].Nodes[1])
	}
}
