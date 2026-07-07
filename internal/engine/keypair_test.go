package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
)

func TestEnsureKeyPair_CreatesAndSavesPem(t *testing.T) {
	pemPath := filepath.Join(t.TempDir(), "isuenv.pem")
	m := &awsapi.Mock{
		DescribeKeyPairsFunc: func(ctx context.Context, in *ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error) {
			return &ec2.DescribeKeyPairsOutput{}, nil
		},
		CreateKeyPairFunc: func(ctx context.Context, in *ec2.CreateKeyPairInput) (*ec2.CreateKeyPairOutput, error) {
			if aws.ToString(in.KeyName) != "isuenv" {
				t.Errorf("unexpected key name: %v", in.KeyName)
			}
			return &ec2.CreateKeyPairOutput{KeyMaterial: aws.String("PRIVATE-KEY-MATERIAL")}, nil
		},
	}
	e := &Engine{EC2: m}
	name, err := e.EnsureKeyPair(context.Background(), pemPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "isuenv" {
		t.Errorf("unexpected key name: %s", name)
	}
	data, err := os.ReadFile(pemPath)
	if err != nil {
		t.Fatalf("pem not written: %v", err)
	}
	if string(data) != "PRIVATE-KEY-MATERIAL" {
		t.Errorf("unexpected pem content: %s", data)
	}
	info, _ := os.Stat(pemPath)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("pem must be 0600, got %v", info.Mode().Perm())
	}
}

func TestEnsureKeyPair_ExistingWithPem(t *testing.T) {
	pemPath := filepath.Join(t.TempDir(), "isuenv.pem")
	if err := os.WriteFile(pemPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &awsapi.Mock{
		DescribeKeyPairsFunc: func(ctx context.Context, in *ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error) {
			return &ec2.DescribeKeyPairsOutput{KeyPairs: []ec2types.KeyPairInfo{{KeyName: aws.String("isuenv")}}}, nil
		},
	}
	e := &Engine{EC2: m}
	name, err := e.EnsureKeyPair(context.Background(), pemPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "isuenv" {
		t.Errorf("unexpected key name: %s", name)
	}
}

func TestEnsureKeyPair_ExistingWithoutPem(t *testing.T) {
	pemPath := filepath.Join(t.TempDir(), "isuenv.pem")
	m := &awsapi.Mock{
		DescribeKeyPairsFunc: func(ctx context.Context, in *ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error) {
			return &ec2.DescribeKeyPairsOutput{KeyPairs: []ec2types.KeyPairInfo{{KeyName: aws.String("isuenv")}}}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.EnsureKeyPair(context.Background(), pemPath)
	if err == nil {
		t.Fatal("expected error when key exists on AWS but pem is missing locally")
	}
	if !strings.Contains(err.Error(), "isuenv nuke") {
		t.Errorf("error should suggest recovery via `isuenv nuke`: %v", err)
	}
}
