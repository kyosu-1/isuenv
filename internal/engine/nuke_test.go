package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
)

func filterIncludes(filters []ec2types.Filter, name, value string) bool {
	for _, f := range filters {
		if aws.ToString(f.Name) != name {
			continue
		}
		for _, v := range f.Values {
			if v == value {
				return true
			}
		}
	}
	return false
}

// isuenv down の直後にnukeすると、インスタンスはまだshutting-downでENIがSGを掴んでいる。
// この状態を検索対象から漏らすと終了を待たずにDeleteSecurityGroupへ進み、
// AWSが DependencyViolation を返してnukeが中途半端に失敗する。
func TestNuke_WaitsForShuttingDownInstances(t *testing.T) {
	PollInterval = time.Millisecond
	state := ec2types.InstanceStateNameShuttingDown
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) > 0 {
				// 終了待ちポーリング。1回目はまだshutting-downで、次のポーリングでterminatedになる。
				current := state
				state = ec2types.InstanceStateNameTerminated
				return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
					InstanceId: aws.String("i-1"),
					State:      &ec2types.InstanceState{Name: current},
				}}}}}, nil
			}
			// AWSと同じく、instance-state-nameフィルタに合致しないインスタンスは返さない。
			if !filterIncludes(in.Filters, "instance-state-name", string(ec2types.InstanceStateNameShuttingDown)) {
				return &ec2.DescribeInstancesOutput{}, nil
			}
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
				InstanceId: aws.String("i-1"),
				State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameShuttingDown},
			}}}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			return &ec2.TerminateInstancesOutput{}, nil
		},
		DescribeKeyPairsFunc: func(ctx context.Context, in *ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error) {
			return &ec2.DescribeKeyPairsOutput{}, nil
		},
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-1")}}}, nil
		},
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1")}}}, nil
		},
		DeleteSecurityGroupFunc: func(ctx context.Context, in *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
			if state != ec2types.InstanceStateNameTerminated {
				return nil, fmt.Errorf("DependencyViolation: resource %s has a dependent object", aws.ToString(in.GroupId))
			}
			return &ec2.DeleteSecurityGroupOutput{}, nil
		},
		DescribeSubnetsFunc: func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{}, nil
		},
		DescribeInternetGatewaysFunc: func(ctx context.Context, in *ec2.DescribeInternetGatewaysInput) (*ec2.DescribeInternetGatewaysOutput, error) {
			return &ec2.DescribeInternetGatewaysOutput{}, nil
		},
		DeleteVpcFunc: func(ctx context.Context, in *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
			return &ec2.DeleteVpcOutput{}, nil
		},
	}
	if err := (&Engine{EC2: m}).Nuke(context.Background()); err != nil {
		t.Fatalf("nuke must wait for shutting-down instances before deleting the security group: %v", err)
	}
}

func TestNuke_DeletesEverything(t *testing.T) {
	PollInterval = time.Millisecond
	deleted := map[string]bool{}
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) > 0 {
				// 終了待ちポーリング: terminated扱い
				return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
					InstanceId: aws.String("i-1"),
					State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameTerminated},
				}}}}}, nil
			}
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
			}}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			deleted["instances"] = true
			return &ec2.TerminateInstancesOutput{}, nil
		},
		DescribeKeyPairsFunc: func(ctx context.Context, in *ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error) {
			return &ec2.DescribeKeyPairsOutput{KeyPairs: []ec2types.KeyPairInfo{{KeyName: aws.String("isuenv")}}}, nil
		},
		DeleteKeyPairFunc: func(ctx context.Context, in *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error) {
			deleted["keypair"] = true
			return &ec2.DeleteKeyPairOutput{}, nil
		},
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-1")}}}, nil
		},
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1")}}}, nil
		},
		DeleteSecurityGroupFunc: func(ctx context.Context, in *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
			deleted["sg"] = true
			return &ec2.DeleteSecurityGroupOutput{}, nil
		},
		DescribeSubnetsFunc: func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{Subnets: []ec2types.Subnet{{SubnetId: aws.String("subnet-1")}}}, nil
		},
		DeleteSubnetFunc: func(ctx context.Context, in *ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error) {
			deleted["subnet"] = true
			return &ec2.DeleteSubnetOutput{}, nil
		},
		DescribeInternetGatewaysFunc: func(ctx context.Context, in *ec2.DescribeInternetGatewaysInput) (*ec2.DescribeInternetGatewaysOutput, error) {
			return &ec2.DescribeInternetGatewaysOutput{InternetGateways: []ec2types.InternetGateway{{InternetGatewayId: aws.String("igw-1")}}}, nil
		},
		DetachInternetGatewayFunc: func(ctx context.Context, in *ec2.DetachInternetGatewayInput) (*ec2.DetachInternetGatewayOutput, error) {
			deleted["igw-detach"] = true
			return &ec2.DetachInternetGatewayOutput{}, nil
		},
		DeleteInternetGatewayFunc: func(ctx context.Context, in *ec2.DeleteInternetGatewayInput) (*ec2.DeleteInternetGatewayOutput, error) {
			deleted["igw"] = true
			return &ec2.DeleteInternetGatewayOutput{}, nil
		},
		DeleteVpcFunc: func(ctx context.Context, in *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
			deleted["vpc"] = true
			return &ec2.DeleteVpcOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	if err := e.Nuke(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"instances", "keypair", "sg", "subnet", "igw-detach", "igw", "vpc"} {
		if !deleted[key] {
			t.Errorf("nuke must delete %s", key)
		}
	}
}
