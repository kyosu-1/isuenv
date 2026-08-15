package engine

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Nuke はisuenv管理下の全リソース（インスタンス・キーペア・SG・サブネット・IGW・VPC）を削除する。
func (e *Engine) Nuke(ctx context.Context) error {
	out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			// shutting-downも対象に含める。`isuenv down`直後のインスタンスはこの状態で、
			// 見落とすと終了を待たずにSG削除へ進みDependencyViolationになる。
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped", "shutting-down"}},
		},
	})
	if err != nil {
		return fmt.Errorf("describe instances: %w", err)
	}
	var ids []string
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			ids = append(ids, aws.ToString(inst.InstanceId))
		}
	}
	if len(ids) > 0 {
		if _, err := e.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids}); err != nil {
			return fmt.Errorf("terminate instances: %w", err)
		}
		if err := e.waitTerminated(ctx, ids); err != nil {
			return err
		}
	}

	// キー名だけでなくisuenv管理タグでも絞り込んでから削除する。
	// 名前だけで消すと、たまたま同名(isuenv)で作られた無関係のキーペアまで消してしまう。
	keyPairs, err := e.EC2.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{
		Filters: []ec2types.Filter{managedFilter(), {Name: aws.String("key-name"), Values: []string{KeyName}}},
	})
	if err != nil {
		return fmt.Errorf("describe key pairs: %w", err)
	}
	if len(keyPairs.KeyPairs) > 0 {
		// 存在しなくてもエラーにしない（冪等性のため無視）
		_, _ = e.EC2.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(KeyName)})
	}

	vpcs, err := e.EC2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{Filters: []ec2types.Filter{managedFilter()}})
	if err != nil {
		return fmt.Errorf("describe vpcs: %w", err)
	}
	for _, vpc := range vpcs.Vpcs {
		vpcID := aws.ToString(vpc.VpcId)
		vpcFilter := ec2types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}}

		sgs, err := e.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{Filters: []ec2types.Filter{managedFilter(), vpcFilter}})
		if err != nil {
			return fmt.Errorf("describe security groups: %w", err)
		}
		for _, sg := range sgs.SecurityGroups {
			if _, err := e.EC2.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: sg.GroupId}); err != nil {
				return fmt.Errorf("delete security group %s: %w", aws.ToString(sg.GroupId), err)
			}
		}

		subnets, err := e.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{Filters: []ec2types.Filter{managedFilter(), vpcFilter}})
		if err != nil {
			return fmt.Errorf("describe subnets: %w", err)
		}
		for _, subnet := range subnets.Subnets {
			if _, err := e.EC2.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: subnet.SubnetId}); err != nil {
				return fmt.Errorf("delete subnet %s: %w", aws.ToString(subnet.SubnetId), err)
			}
		}

		igws, err := e.EC2.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
			Filters: []ec2types.Filter{{Name: aws.String("attachment.vpc-id"), Values: []string{vpcID}}},
		})
		if err != nil {
			return fmt.Errorf("describe internet gateways: %w", err)
		}
		for _, igw := range igws.InternetGateways {
			if _, err := e.EC2.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
				InternetGatewayId: igw.InternetGatewayId,
				VpcId:             aws.String(vpcID),
			}); err != nil {
				return fmt.Errorf("detach internet gateway: %w", err)
			}
			if _, err := e.EC2.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{InternetGatewayId: igw.InternetGatewayId}); err != nil {
				return fmt.Errorf("delete internet gateway: %w", err)
			}
		}

		if _, err := e.EC2.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: vpc.VpcId}); err != nil {
			return fmt.Errorf("delete vpc %s: %w", vpcID, err)
		}
	}
	return nil
}
