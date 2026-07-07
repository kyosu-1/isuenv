package engine

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const KeyName = "isuenv"

// EnsureKeyPair はisuenv用キーペアを確保し、キー名を返す。
// 名前指定のDescribeKeyPairsは存在しない場合エラーになるため、Filterで検索する。
func (e *Engine) EnsureKeyPair(ctx context.Context, pemPath string) (string, error) {
	out, err := e.EC2.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{
		Filters: []ec2types.Filter{{Name: aws.String("key-name"), Values: []string{KeyName}}},
	})
	if err != nil {
		return "", fmt.Errorf("describe key pairs: %w", err)
	}
	if len(out.KeyPairs) > 0 {
		if _, statErr := os.Stat(pemPath); statErr == nil {
			return KeyName, nil
		}
		return "", fmt.Errorf("key pair %q exists on AWS but %s is missing locally; run `isuenv nuke` to recreate everything, or restore the pem file", KeyName, pemPath)
	}
	created, err := e.EC2.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{
		KeyName:           aws.String(KeyName),
		TagSpecifications: []ec2types.TagSpecification{managedTagSpec(ec2types.ResourceTypeKeyPair)},
	})
	if err != nil {
		return "", fmt.Errorf("create key pair: %w", err)
	}
	if err := os.WriteFile(pemPath, []byte(aws.ToString(created.KeyMaterial)), 0o600); err != nil {
		return "", fmt.Errorf("write pem to %s: %w", pemPath, err)
	}
	return KeyName, nil
}
