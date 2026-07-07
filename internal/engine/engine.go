// Package engine はisuenvのAWSオーケストレーションを実装する。
// 状態はローカルに持たず、isuenv:* タグでAWS上のリソースを識別する。
package engine

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
)

const (
	TagManaged = "isuenv:managed"
	TagEnv     = "isuenv:env"
	TagNode    = "isuenv:node"
	TagExpires = "isuenv:expires-at"
)

type Engine struct {
	EC2 awsapi.EC2API
}

func managedFilter() ec2types.Filter {
	return ec2types.Filter{Name: aws.String("tag:" + TagManaged), Values: []string{"true"}}
}

func tagValue(tags []ec2types.Tag, key string) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == key {
			return aws.ToString(t.Value)
		}
	}
	return ""
}
