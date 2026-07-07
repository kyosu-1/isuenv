package engine

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
)

func TestEnsureNetwork_ReusesExisting(t *testing.T) {
	m := &awsapi.Mock{
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-exists")}}}, nil
		},
		DescribeSubnetsFunc: func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{Subnets: []ec2types.Subnet{{SubnetId: aws.String("subnet-exists")}}}, nil
		},
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-exists")}}}, nil
		},
	}
	e := &Engine{EC2: m}
	net, err := e.EnsureNetwork(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Network{VpcID: "vpc-exists", SubnetID: "subnet-exists", SecurityGroupID: "sg-exists"}
	if net != want {
		t.Errorf("expected %+v, got %+v", want, net)
	}
}

func TestEnsureNetwork_CreatesWhenAbsent(t *testing.T) {
	var routeIn *ec2.CreateRouteInput
	m := &awsapi.Mock{
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{}, nil
		},
		CreateVpcFunc: func(ctx context.Context, in *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
			return &ec2.CreateVpcOutput{Vpc: &ec2types.Vpc{VpcId: aws.String("vpc-new")}}, nil
		},
		CreateSubnetFunc: func(ctx context.Context, in *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
			if aws.ToString(in.VpcId) != "vpc-new" {
				t.Errorf("subnet must be created in vpc-new: %v", in.VpcId)
			}
			return &ec2.CreateSubnetOutput{Subnet: &ec2types.Subnet{SubnetId: aws.String("subnet-new")}}, nil
		},
		ModifySubnetAttributeFunc: func(ctx context.Context, in *ec2.ModifySubnetAttributeInput) (*ec2.ModifySubnetAttributeOutput, error) {
			if in.MapPublicIpOnLaunch == nil || !aws.ToBool(in.MapPublicIpOnLaunch.Value) {
				t.Error("subnet must map public IPs on launch")
			}
			return &ec2.ModifySubnetAttributeOutput{}, nil
		},
		CreateInternetGatewayFunc: func(ctx context.Context, in *ec2.CreateInternetGatewayInput) (*ec2.CreateInternetGatewayOutput, error) {
			return &ec2.CreateInternetGatewayOutput{InternetGateway: &ec2types.InternetGateway{InternetGatewayId: aws.String("igw-new")}}, nil
		},
		AttachInternetGatewayFunc: func(ctx context.Context, in *ec2.AttachInternetGatewayInput) (*ec2.AttachInternetGatewayOutput, error) {
			return &ec2.AttachInternetGatewayOutput{}, nil
		},
		DescribeRouteTablesFunc: func(ctx context.Context, in *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
			return &ec2.DescribeRouteTablesOutput{RouteTables: []ec2types.RouteTable{{RouteTableId: aws.String("rtb-main")}}}, nil
		},
		CreateRouteFunc: func(ctx context.Context, in *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
			routeIn = in
			return &ec2.CreateRouteOutput{}, nil
		},
		CreateSecurityGroupFunc: func(ctx context.Context, in *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
			return &ec2.CreateSecurityGroupOutput{GroupId: aws.String("sg-new")}, nil
		},
	}
	e := &Engine{EC2: m}
	net, err := e.EnsureNetwork(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Network{VpcID: "vpc-new", SubnetID: "subnet-new", SecurityGroupID: "sg-new"}
	if net != want {
		t.Errorf("expected %+v, got %+v", want, net)
	}
	if routeIn == nil || aws.ToString(routeIn.DestinationCidrBlock) != "0.0.0.0/0" || aws.ToString(routeIn.GatewayId) != "igw-new" {
		t.Errorf("default route to igw-new required: %+v", routeIn)
	}
}

func TestFindManagedSecurityGroup_Found(t *testing.T) {
	m := &awsapi.Mock{
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1")}}}, nil
		},
	}
	e := &Engine{EC2: m}
	id, err := e.FindManagedSecurityGroup(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "sg-1" {
		t.Errorf("expected sg-1, got %q", id)
	}
}

func TestFindManagedSecurityGroup_None(t *testing.T) {
	m := &awsapi.Mock{
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	id, err := e.FindManagedSecurityGroup(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
}

func TestEnsureIngress_ReplacesRulesWithMyIP(t *testing.T) {
	existing := []ec2types.IpPermission{{
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(22), ToPort: aws.Int32(22),
		IpRanges: []ec2types.IpRange{{CidrIp: aws.String("203.0.113.9/32")}},
	}}
	var revoked *ec2.RevokeSecurityGroupIngressInput
	var authorized *ec2.AuthorizeSecurityGroupIngressInput
	m := &awsapi.Mock{
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1"), IpPermissions: existing}}}, nil
		},
		RevokeSecurityGroupIngressFunc: func(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
			revoked = in
			return &ec2.RevokeSecurityGroupIngressOutput{}, nil
		},
		AuthorizeSecurityGroupIngressFunc: func(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
			authorized = in
			return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	if err := e.EnsureIngress(context.Background(), "sg-1", "198.51.100.7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked == nil || len(revoked.IpPermissions) != 1 {
		t.Fatalf("existing rules must be revoked: %+v", revoked)
	}
	if authorized == nil || len(authorized.IpPermissions) != 3 {
		t.Fatalf("expected 3 rules (22/80/443): %+v", authorized)
	}
	for _, perm := range authorized.IpPermissions {
		if aws.ToString(perm.IpRanges[0].CidrIp) != "198.51.100.7/32" {
			t.Errorf("rules must be scoped to my ip: %+v", perm)
		}
	}
}
