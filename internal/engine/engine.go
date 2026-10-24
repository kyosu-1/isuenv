// Package engine はisuenvのAWSオーケストレーションを実装する。
// 状態はローカルに持たず、isuenv:* タグでAWS上のリソースを識別する。
package engine

import (
	"fmt"

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
	// TagVCPUs / TagMemGB は起動時にかけた制限(CpuOptions で絞った後のvCPU数、mem= のGiB数)。
	// 制限をかけたノードにだけ付ける。メモリ制限はインスタンスの外から見えないので、
	// タグに残しておかないと list などで表示できない。
	TagVCPUs = "isuenv:vcpus"
	TagMemGB = "isuenv:mem-gb"
	// TagProblem は環境の問題名。環境名は --name で問題名と別にできるので、
	// ssh のユーザーやベンチの起動方法を引くために問題名を別に残す。
	TagProblem = "isuenv:problem"
	// TagHosts は環境の全ノードのプライベートIP。"1=10.100.0.5,2=10.100.0.6,bench=10.100.0.7" の形。
	// 全ノードを起動した後に付け、ノードはインスタンスメタデータのタグ(IMDS)から読む。
	// 台ごとに役割が違う問題(isucon7-final の Web 3台 + DB 1台)が、他のノードのIPを知るのに使う。
	// IPは起動するまで決まらないので user-data には書けない。
	TagHosts = "isuenv:hosts"
)

// isuenv:role タグの値。
const (
	RoleApp   = "app"
	RoleBench = "bench"
)

// NodeName はノードの名前を返す。EC2のNameタグ、sshのホスト名、CLIの表示で共通に使う。
// 競技ノードは <環境名>-<番号>、ベンチノードは <環境名>-bench。ベンチノードは1環境に
// 1台しか作れないので番号は要らず、番号付きの名前は競技ノードだけ、と見分けられる。
func NodeName(env string, index int, role string) string {
	if role == RoleBench {
		return env + "-bench"
	}
	return fmt.Sprintf("%s-%d", env, index)
}

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
