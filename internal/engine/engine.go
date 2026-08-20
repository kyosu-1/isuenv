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
	// TagRole は競技用ノードとベンチマーカー用ノードを区別する。
	// 両者はインスタンスタイプが異なりうるので、タグに残しておかないと
	// ローカルに状態を持たないCLIからは後で見分けられない。
	TagRole = "isuenv:role"
)

// isuenv:role タグの値。
const (
	RoleApp   = "app"
	RoleBench = "bench"
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
