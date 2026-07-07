package engine

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

func TestResolveAMI_PicksLatestAndSetsRequiredParams(t *testing.T) {
	var got *ec2.DescribeImagesInput
	m := &awsapi.Mock{
		DescribeImagesFunc: func(ctx context.Context, in *ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error) {
			got = in
			return &ec2.DescribeImagesOutput{Images: []ec2types.Image{
				{ImageId: aws.String("ami-old"), CreationDate: aws.String("2023-11-28T10:22:23.000Z")},
				{ImageId: aws.String("ami-new"), CreationDate: aws.String("2024-12-11T11:26:39.000Z")},
			}}, nil
		},
	}
	e := &Engine{EC2: m}
	p := catalog.Problem{Name: "isucon14", AMIPattern: "isucon14-*", OwnerID: "839726181030"}

	id, err := e.ResolveAMI(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ami-new" {
		t.Errorf("expected latest ami-new, got %s", id)
	}
	if len(got.Owners) != 1 || got.Owners[0] != "839726181030" {
		t.Errorf("owner must be pinned: %v", got.Owners)
	}
	if got.IncludeDeprecated == nil || !*got.IncludeDeprecated {
		t.Error("IncludeDeprecated must be true (older ISUCON AMIs are deprecated)")
	}
}

func TestResolveAMI_NoImage(t *testing.T) {
	m := &awsapi.Mock{
		DescribeImagesFunc: func(ctx context.Context, in *ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error) {
			return &ec2.DescribeImagesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.ResolveAMI(context.Background(), catalog.Problem{Name: "isucon13", AMIPattern: "isucon13-*", OwnerID: "839726181030"})
	if err == nil {
		t.Fatal("expected error when no AMI found")
	}
}
