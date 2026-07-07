package engine

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	vpcCIDR    = "10.100.0.0/16"
	subnetCIDR = "10.100.0.0/24"
	sgName     = "isuenv"
)

type Network struct {
	VpcID           string
	SubnetID        string
	SecurityGroupID string
}

func managedTagSpec(resType ec2types.ResourceType) ec2types.TagSpecification {
	return ec2types.TagSpecification{
		ResourceType: resType,
		Tags: []ec2types.Tag{
			{Key: aws.String(TagManaged), Value: aws.String("true")},
			{Key: aws.String("Name"), Value: aws.String("isuenv")},
		},
	}
}

// EnsureNetwork はisuenv専用のVPC/パブリックサブネット/SGを検索し、なければ作成する。
func (e *Engine) EnsureNetwork(ctx context.Context) (Network, error) {
	vpcs, err := e.EC2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{Filters: []ec2types.Filter{managedFilter()}})
	if err != nil {
		return Network{}, fmt.Errorf("describe vpcs: %w", err)
	}
	if len(vpcs.Vpcs) > 0 {
		return e.describeExistingNetwork(ctx, aws.ToString(vpcs.Vpcs[0].VpcId))
	}
	return e.createNetwork(ctx)
}

func (e *Engine) describeExistingNetwork(ctx context.Context, vpcID string) (Network, error) {
	vpcFilter := ec2types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}}
	subnets, err := e.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{Filters: []ec2types.Filter{managedFilter(), vpcFilter}})
	if err != nil {
		return Network{}, fmt.Errorf("describe subnets: %w", err)
	}
	if len(subnets.Subnets) == 0 {
		return Network{}, fmt.Errorf("isuenv vpc %s exists but has no managed subnet; run `isuenv nuke` and retry", vpcID)
	}
	sgs, err := e.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{Filters: []ec2types.Filter{managedFilter(), vpcFilter}})
	if err != nil {
		return Network{}, fmt.Errorf("describe security groups: %w", err)
	}
	if len(sgs.SecurityGroups) == 0 {
		return Network{}, fmt.Errorf("isuenv vpc %s exists but has no managed security group; run `isuenv nuke` and retry", vpcID)
	}
	return Network{
		VpcID:           vpcID,
		SubnetID:        aws.ToString(subnets.Subnets[0].SubnetId),
		SecurityGroupID: aws.ToString(sgs.SecurityGroups[0].GroupId),
	}, nil
}

func (e *Engine) createNetwork(ctx context.Context) (Network, error) {
	vpcOut, err := e.EC2.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock:         aws.String(vpcCIDR),
		TagSpecifications: []ec2types.TagSpecification{managedTagSpec(ec2types.ResourceTypeVpc)},
	})
	if err != nil {
		return Network{}, fmt.Errorf("create vpc: %w", err)
	}
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)

	subnetOut, err := e.EC2.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:             aws.String(vpcID),
		CidrBlock:         aws.String(subnetCIDR),
		TagSpecifications: []ec2types.TagSpecification{managedTagSpec(ec2types.ResourceTypeSubnet)},
	})
	if err != nil {
		return Network{}, fmt.Errorf("create subnet: %w", err)
	}
	subnetID := aws.ToString(subnetOut.Subnet.SubnetId)

	if _, err := e.EC2.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
		SubnetId:            aws.String(subnetID),
		MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
	}); err != nil {
		return Network{}, fmt.Errorf("enable public ip on subnet: %w", err)
	}

	igwOut, err := e.EC2.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: []ec2types.TagSpecification{managedTagSpec(ec2types.ResourceTypeInternetGateway)},
	})
	if err != nil {
		return Network{}, fmt.Errorf("create internet gateway: %w", err)
	}
	igwID := aws.ToString(igwOut.InternetGateway.InternetGatewayId)

	if _, err := e.EC2.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(igwID),
		VpcId:             aws.String(vpcID),
	}); err != nil {
		return Network{}, fmt.Errorf("attach internet gateway: %w", err)
	}

	rts, err := e.EC2.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("association.main"), Values: []string{"true"}},
		},
	})
	if err != nil {
		return Network{}, fmt.Errorf("describe route tables: %w", err)
	}
	if len(rts.RouteTables) == 0 {
		return Network{}, fmt.Errorf("main route table not found for vpc %s", vpcID)
	}
	if _, err := e.EC2.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         rts.RouteTables[0].RouteTableId,
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
	}); err != nil {
		return Network{}, fmt.Errorf("create default route: %w", err)
	}

	sgOut, err := e.EC2.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:         aws.String(sgName),
		Description:       aws.String("isuenv practice environments"),
		VpcId:             aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{managedTagSpec(ec2types.ResourceTypeSecurityGroup)},
	})
	if err != nil {
		return Network{}, fmt.Errorf("create security group: %w", err)
	}

	return Network{VpcID: vpcID, SubnetID: subnetID, SecurityGroupID: aws.ToString(sgOut.GroupId)}, nil
}

// FindManagedSecurityGroup はisuenv管理下のSGを検索する。
// EnsureNetworkと異なりVPCなどのリソースを作成しない（ssh実行時にネットワークを新規作成しないため）。
// 見つからない場合は空文字列とnilを返す。
func (e *Engine) FindManagedSecurityGroup(ctx context.Context) (string, error) {
	sgs, err := e.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{Filters: []ec2types.Filter{managedFilter()}})
	if err != nil {
		return "", fmt.Errorf("describe security groups: %w", err)
	}
	if len(sgs.SecurityGroups) == 0 {
		return "", nil
	}
	return aws.ToString(sgs.SecurityGroups[0].GroupId), nil
}

// EnsureIngress はSGのingressを「myIP/32からのtcp 22/80/443のみ」に置き換える。
func (e *Engine) EnsureIngress(ctx context.Context, sgID, myIP string) error {
	out, err := e.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{sgID}})
	if err != nil {
		return fmt.Errorf("describe security group %s: %w", sgID, err)
	}
	if len(out.SecurityGroups) == 0 {
		return fmt.Errorf("security group %s not found", sgID)
	}
	if existing := out.SecurityGroups[0].IpPermissions; len(existing) > 0 {
		if _, err := e.EC2.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(sgID),
			IpPermissions: existing,
		}); err != nil {
			return fmt.Errorf("revoke old ingress: %w", err)
		}
	}
	cidr := myIP + "/32"
	var perms []ec2types.IpPermission
	for _, port := range []int32{22, 80, 443} {
		perms = append(perms, ec2types.IpPermission{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(port),
			ToPort:     aws.Int32(port),
			IpRanges:   []ec2types.IpRange{{CidrIp: aws.String(cidr), Description: aws.String("isuenv")}},
		})
	}
	// ノード間通信用: SG自身からの全プロトコル許可。--nodes>1構成でプライベートIP経由の疎通に必要。
	// EnsureIngressはup/ssh実行のたびにルールを総入れ替えするため、createNetworkではなくここに置いて維持する。
	perms = append(perms, ec2types.IpPermission{
		IpProtocol:       aws.String("-1"),
		UserIdGroupPairs: []ec2types.UserIdGroupPair{{GroupId: aws.String(sgID), Description: aws.String("isuenv node-to-node")}},
	})
	if _, err := e.EC2.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       aws.String(sgID),
		IpPermissions: perms,
	}); err != nil {
		return fmt.Errorf("authorize ingress from %s: %w", cidr, err)
	}
	return nil
}
