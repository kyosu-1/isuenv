package awsapi

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Mock は EC2API の手書きモック。使うメソッドのFuncフィールドだけ設定する。
// 未設定のメソッドが呼ばれた場合はpanicし、テストで想定外の呼び出しを検出する。
type Mock struct {
	DescribeImagesFunc                func(ctx context.Context, in *ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error)
	DescribeVpcsFunc                  func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
	CreateVpcFunc                     func(ctx context.Context, in *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error)
	DeleteVpcFunc                     func(ctx context.Context, in *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error)
	DescribeSubnetsFunc               func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	CreateSubnetFunc                  func(ctx context.Context, in *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error)
	ModifySubnetAttributeFunc         func(ctx context.Context, in *ec2.ModifySubnetAttributeInput) (*ec2.ModifySubnetAttributeOutput, error)
	DeleteSubnetFunc                  func(ctx context.Context, in *ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error)
	DescribeInternetGatewaysFunc      func(ctx context.Context, in *ec2.DescribeInternetGatewaysInput) (*ec2.DescribeInternetGatewaysOutput, error)
	CreateInternetGatewayFunc         func(ctx context.Context, in *ec2.CreateInternetGatewayInput) (*ec2.CreateInternetGatewayOutput, error)
	AttachInternetGatewayFunc         func(ctx context.Context, in *ec2.AttachInternetGatewayInput) (*ec2.AttachInternetGatewayOutput, error)
	DetachInternetGatewayFunc         func(ctx context.Context, in *ec2.DetachInternetGatewayInput) (*ec2.DetachInternetGatewayOutput, error)
	DeleteInternetGatewayFunc         func(ctx context.Context, in *ec2.DeleteInternetGatewayInput) (*ec2.DeleteInternetGatewayOutput, error)
	DescribeRouteTablesFunc           func(ctx context.Context, in *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error)
	CreateRouteFunc                   func(ctx context.Context, in *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error)
	DescribeSecurityGroupsFunc        func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	CreateSecurityGroupFunc           func(ctx context.Context, in *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngressFunc func(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngressFunc    func(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error)
	DeleteSecurityGroupFunc           func(ctx context.Context, in *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
	DescribeKeyPairsFunc              func(ctx context.Context, in *ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error)
	CreateKeyPairFunc                 func(ctx context.Context, in *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error)
	DeleteKeyPairFunc                 func(ctx context.Context, in *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error)
	RunInstancesFunc                  func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error)
	DescribeInstancesFunc             func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
	TerminateInstancesFunc            func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)
}

func (m *Mock) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if m.DescribeImagesFunc == nil {
		panic("unexpected call: DescribeImages")
	}
	return m.DescribeImagesFunc(ctx, in)
}

func (m *Mock) DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if m.DescribeVpcsFunc == nil {
		panic("unexpected call: DescribeVpcs")
	}
	return m.DescribeVpcsFunc(ctx, in)
}

func (m *Mock) CreateVpc(ctx context.Context, in *ec2.CreateVpcInput, _ ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
	if m.CreateVpcFunc == nil {
		panic("unexpected call: CreateVpc")
	}
	return m.CreateVpcFunc(ctx, in)
}

func (m *Mock) DeleteVpc(ctx context.Context, in *ec2.DeleteVpcInput, _ ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error) {
	if m.DeleteVpcFunc == nil {
		panic("unexpected call: DeleteVpc")
	}
	return m.DeleteVpcFunc(ctx, in)
}

func (m *Mock) DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if m.DescribeSubnetsFunc == nil {
		panic("unexpected call: DescribeSubnets")
	}
	return m.DescribeSubnetsFunc(ctx, in)
}

func (m *Mock) CreateSubnet(ctx context.Context, in *ec2.CreateSubnetInput, _ ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
	if m.CreateSubnetFunc == nil {
		panic("unexpected call: CreateSubnet")
	}
	return m.CreateSubnetFunc(ctx, in)
}

func (m *Mock) ModifySubnetAttribute(ctx context.Context, in *ec2.ModifySubnetAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error) {
	if m.ModifySubnetAttributeFunc == nil {
		panic("unexpected call: ModifySubnetAttribute")
	}
	return m.ModifySubnetAttributeFunc(ctx, in)
}

func (m *Mock) DeleteSubnet(ctx context.Context, in *ec2.DeleteSubnetInput, _ ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
	if m.DeleteSubnetFunc == nil {
		panic("unexpected call: DeleteSubnet")
	}
	return m.DeleteSubnetFunc(ctx, in)
}

func (m *Mock) DescribeInternetGateways(ctx context.Context, in *ec2.DescribeInternetGatewaysInput, _ ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error) {
	if m.DescribeInternetGatewaysFunc == nil {
		panic("unexpected call: DescribeInternetGateways")
	}
	return m.DescribeInternetGatewaysFunc(ctx, in)
}

func (m *Mock) CreateInternetGateway(ctx context.Context, in *ec2.CreateInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error) {
	if m.CreateInternetGatewayFunc == nil {
		panic("unexpected call: CreateInternetGateway")
	}
	return m.CreateInternetGatewayFunc(ctx, in)
}

func (m *Mock) AttachInternetGateway(ctx context.Context, in *ec2.AttachInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error) {
	if m.AttachInternetGatewayFunc == nil {
		panic("unexpected call: AttachInternetGateway")
	}
	return m.AttachInternetGatewayFunc(ctx, in)
}

func (m *Mock) DetachInternetGateway(ctx context.Context, in *ec2.DetachInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error) {
	if m.DetachInternetGatewayFunc == nil {
		panic("unexpected call: DetachInternetGateway")
	}
	return m.DetachInternetGatewayFunc(ctx, in)
}

func (m *Mock) DeleteInternetGateway(ctx context.Context, in *ec2.DeleteInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error) {
	if m.DeleteInternetGatewayFunc == nil {
		panic("unexpected call: DeleteInternetGateway")
	}
	return m.DeleteInternetGatewayFunc(ctx, in)
}

func (m *Mock) DescribeRouteTables(ctx context.Context, in *ec2.DescribeRouteTablesInput, _ ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	if m.DescribeRouteTablesFunc == nil {
		panic("unexpected call: DescribeRouteTables")
	}
	return m.DescribeRouteTablesFunc(ctx, in)
}

func (m *Mock) CreateRoute(ctx context.Context, in *ec2.CreateRouteInput, _ ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	if m.CreateRouteFunc == nil {
		panic("unexpected call: CreateRoute")
	}
	return m.CreateRouteFunc(ctx, in)
}

func (m *Mock) DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if m.DescribeSecurityGroupsFunc == nil {
		panic("unexpected call: DescribeSecurityGroups")
	}
	return m.DescribeSecurityGroupsFunc(ctx, in)
}

func (m *Mock) CreateSecurityGroup(ctx context.Context, in *ec2.CreateSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	if m.CreateSecurityGroupFunc == nil {
		panic("unexpected call: CreateSecurityGroup")
	}
	return m.CreateSecurityGroupFunc(ctx, in)
}

func (m *Mock) AuthorizeSecurityGroupIngress(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	if m.AuthorizeSecurityGroupIngressFunc == nil {
		panic("unexpected call: AuthorizeSecurityGroupIngress")
	}
	return m.AuthorizeSecurityGroupIngressFunc(ctx, in)
}

func (m *Mock) RevokeSecurityGroupIngress(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	if m.RevokeSecurityGroupIngressFunc == nil {
		panic("unexpected call: RevokeSecurityGroupIngress")
	}
	return m.RevokeSecurityGroupIngressFunc(ctx, in)
}

func (m *Mock) DeleteSecurityGroup(ctx context.Context, in *ec2.DeleteSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	if m.DeleteSecurityGroupFunc == nil {
		panic("unexpected call: DeleteSecurityGroup")
	}
	return m.DeleteSecurityGroupFunc(ctx, in)
}

func (m *Mock) DescribeKeyPairs(ctx context.Context, in *ec2.DescribeKeyPairsInput, _ ...func(*ec2.Options)) (*ec2.DescribeKeyPairsOutput, error) {
	if m.DescribeKeyPairsFunc == nil {
		panic("unexpected call: DescribeKeyPairs")
	}
	return m.DescribeKeyPairsFunc(ctx, in)
}

func (m *Mock) CreateKeyPair(ctx context.Context, in *ec2.CreateKeyPairInput, _ ...func(*ec2.Options)) (*ec2.CreateKeyPairOutput, error) {
	if m.CreateKeyPairFunc == nil {
		panic("unexpected call: CreateKeyPair")
	}
	return m.CreateKeyPairFunc(ctx, in)
}

func (m *Mock) DeleteKeyPair(ctx context.Context, in *ec2.DeleteKeyPairInput, _ ...func(*ec2.Options)) (*ec2.DeleteKeyPairOutput, error) {
	if m.DeleteKeyPairFunc == nil {
		panic("unexpected call: DeleteKeyPair")
	}
	return m.DeleteKeyPairFunc(ctx, in)
}

func (m *Mock) RunInstances(ctx context.Context, in *ec2.RunInstancesInput, _ ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	if m.RunInstancesFunc == nil {
		panic("unexpected call: RunInstances")
	}
	return m.RunInstancesFunc(ctx, in)
}

func (m *Mock) DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if m.DescribeInstancesFunc == nil {
		panic("unexpected call: DescribeInstances")
	}
	return m.DescribeInstancesFunc(ctx, in)
}

func (m *Mock) TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	if m.TerminateInstancesFunc == nil {
		panic("unexpected call: TerminateInstances")
	}
	return m.TerminateInstancesFunc(ctx, in)
}

var _ EC2API = (*Mock)(nil)
