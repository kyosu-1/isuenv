package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

// AMI は解決したAMIのIDと名前。名前はどのイメージで起動したかを利用者に示すために持つ。
// 上流(matsuu/aws-isucon)は同じ名前パターンのままAMIを差し替えるので、IDだけでは
// 手元がどの世代を掴んでいるか分からない。
type AMI struct {
	ID   string
	Name string
}

// ResolveAMI は問題のAMI名パターンと所有者から最新のAMIを解決する。
// 古い過去問AMIはdeprecated扱いのため IncludeDeprecated が必須。
func (e *Engine) ResolveAMI(ctx context.Context, p catalog.Problem) (AMI, error) {
	out, err := e.EC2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners:            []string{p.OwnerID},
		IncludeDeprecated: aws.Bool(true),
		Filters: []ec2types.Filter{
			{Name: aws.String("name"), Values: []string{p.AMIPattern}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return AMI{}, fmt.Errorf("describe images for %s: %w", p.Name, err)
	}
	if len(out.Images) == 0 {
		return AMI{}, fmt.Errorf("no AMI found for %s (owner %s, pattern %s)", p.Name, p.OwnerID, p.AMIPattern)
	}
	sort.Slice(out.Images, func(i, j int) bool {
		return aws.ToString(out.Images[i].CreationDate) > aws.ToString(out.Images[j].CreationDate)
	})
	return AMI{
		ID:   aws.ToString(out.Images[0].ImageId),
		Name: aws.ToString(out.Images[0].Name),
	}, nil
}
