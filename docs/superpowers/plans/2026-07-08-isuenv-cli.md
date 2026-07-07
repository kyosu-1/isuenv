# isuenv CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ISUCON過去問の練習環境（matsuu/aws-isucon 公開AMI）をAWS EC2上にコマンド一発で構築・破棄するGo製CLI `isuenv` を作る。

**Architecture:** cobraベースのCLI。AWS操作は `internal/awsapi.EC2API`（EC2 APIの狭いインターフェース）に切り出し、オーケストレーションは `internal/engine` に集約。状態はローカルに持たず、全リソースをタグ（`isuenv:*`）で管理してAWSに問い合わせる。TTLはEC2のuser-data（`shutdown -P`）+ `instance-initiated-shutdown-behavior=terminate` で環境自身に自己消滅させる。

**Tech Stack:** Go 1.26 / spf13/cobra / aws-sdk-go-v2 (config, service/ec2) / gopkg.in/yaml.v3。テストは標準ライブラリのみ（モックは手書き）。

**Spec:** `docs/superpowers/specs/2026-07-08-isuenv-design.md`

## Global Constraints

- モジュールパス: `github.com/kyosu-1/isuenv`
- 依存は cobra / yaml.v3 / aws-sdk-go-v2 (config, service/ec2) のみ。テストに testify 等は入れない
- リージョンは `ap-northeast-1` 固定
- タグキーは正確に: `isuenv:managed` / `isuenv:env` / `isuenv:node` / `isuenv:expires-at`
- AMI所有者アカウントIDは `839726181030`（matsuu氏、全過去問共通。実機確認済み 2026-07-08）
- **AMI解決は必ず `IncludeDeprecated: true` を付ける**（isucon14以外の過去問AMIはdeprecated扱いで、デフォルトでは `DescribeImages` に出てこない。実機確認済み）
- キーペア名は `isuenv`、秘密鍵は `~/.ssh/isuenv.pem`（0600）
- デフォルト値: `--nodes 1` / `--ttl 8h` / `--instance-type c5.large`
- コミットメッセージは conventional commits 形式で、末尾に `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` を付ける
- 各タスクの検証は `go test ./...` と `go build ./...` が通ること

## File Structure

```
isuenv/
├── go.mod / go.sum
├── main.go                          # エントリポイント（cmd.Executeを呼ぶだけ）
├── cmd/
│   ├── root.go                      # rootコマンド、Engine生成、共通ヘルパ
│   ├── problems.go                  # problems コマンド
│   ├── up.go / list.go / ssh.go / down.go / nuke.go
│   └── sshsync.go                   # refreshSSHConfig（listの結果からssh config再生成）
├── internal/
│   ├── catalog/                     # 過去問カタログ（embed YAML）
│   │   ├── catalog.go / catalog.yaml / catalog_test.go
│   ├── awsapi/                      # EC2 APIインターフェースと手書きモック
│   │   ├── api.go / mock.go
│   ├── engine/                      # AWSオーケストレーション本体
│   │   ├── engine.go                # Engine構造体・タグ定数・共通ヘルパ
│   │   ├── ami.go / ami_test.go
│   │   ├── userdata.go / cost.go / userdata_test.go / cost_test.go
│   │   ├── network.go / network_test.go
│   │   ├── keypair.go / keypair_test.go
│   │   ├── instance.go / instance_test.go   # Up/List/Down/Nuke/wait系
│   ├── sshconf/
│   │   ├── sshconf.go / sshconf_test.go
│   └── myip/
│       ├── myip.go / myip_test.go
└── README.md
```

---

### Task 1: プロジェクトスキャフォールド

**Files:**
- Create: `go.mod`, `main.go`, `cmd/root.go`

**Interfaces:**
- Consumes: なし
- Produces: `cmd.Execute() error`（main.goが呼ぶ）、`rootCmd *cobra.Command`（以後のコマンドが `rootCmd.AddCommand` で登録する）

- [ ] **Step 1: モジュール初期化と依存取得**

```bash
cd /Users/abe/ghq/github.com/kyosu-1/isuenv
go mod init github.com/kyosu-1/isuenv
go get github.com/spf13/cobra@latest
go get gopkg.in/yaml.v3@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/ec2@latest
```

- [ ] **Step 2: main.go と cmd/root.go を書く**

`main.go`:

```go
package main

import (
	"os"

	"github.com/kyosu-1/isuenv/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

`cmd/root.go`:

```go
package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/spf13/cobra"
)

const region = "ap-northeast-1"

var rootCmd = &cobra.Command{
	Use:          "isuenv",
	Short:        "ISUCON practice environments on AWS EC2",
	Long:         "isuenv creates and destroys ISUCON practice environments on AWS EC2 using public AMIs from matsuu/aws-isucon.",
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func newEC2Client(ctx context.Context) (*ec2.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}

func sshDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ssh"
	}
	return filepath.Join(home, ".ssh")
}

func pemPath() string {
	return filepath.Join(sshDir(), "isuenv.pem")
}
```

（この時点で `newEC2Client` / `pemPath` は未使用のためコンパイルエラーにはならないが、`go vet` も通ることを確認する）

- [ ] **Step 3: ビルドと動作確認**

Run: `go build -o isuenv . && ./isuenv --help`
Expected: `isuenv creates and destroys ...` を含むヘルプが表示され、exit 0

- [ ] **Step 4: .gitignore とコミット**

`.gitignore`:

```
/isuenv
```

```bash
git add go.mod go.sum main.go cmd/root.go .gitignore
git commit -m "feat: scaffold isuenv CLI with cobra root command

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 過去問カタログ（catalog パッケージ）

**Files:**
- Create: `internal/catalog/catalog.go`, `internal/catalog/catalog.yaml`
- Test: `internal/catalog/catalog_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `type Problem struct { Name, AMIPattern, OwnerID, SSHUser string; DefaultNodes int; Notes string }`
  - `func List() []Problem`
  - `func Lookup(name string) (Problem, error)` — 見つからない場合はエラーメッセージに `isuenv problems` への誘導を含む

- [ ] **Step 1: 失敗するテストを書く**

`internal/catalog/catalog_test.go`:

```go
package catalog

import (
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	problems := List()
	if len(problems) < 10 {
		t.Fatalf("expected at least 10 problems, got %d", len(problems))
	}
	for _, p := range problems {
		if p.Name == "" || p.AMIPattern == "" || p.SSHUser == "" {
			t.Errorf("problem has empty required field: %+v", p)
		}
		if p.OwnerID != "839726181030" {
			t.Errorf("problem %s: unexpected owner id %q", p.Name, p.OwnerID)
		}
		if p.DefaultNodes < 1 {
			t.Errorf("problem %s: default nodes must be >= 1", p.Name)
		}
	}
}

func TestLookup(t *testing.T) {
	p, err := Lookup("isucon13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.AMIPattern != "isucon13-*" {
		t.Errorf("unexpected ami pattern: %q", p.AMIPattern)
	}

	_, err = Lookup("no-such-problem")
	if err == nil {
		t.Fatal("expected error for unknown problem")
	}
	if !strings.Contains(err.Error(), "isuenv problems") {
		t.Errorf("error should mention `isuenv problems`: %v", err)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/catalog/`
Expected: FAIL（`List` / `Lookup` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/catalog/catalog.yaml`:

```yaml
problems:
  - name: isucon9-qualify
    ami_pattern: "isucon9-qualify-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon9-qualify"
  - name: isucon9-final
    ami_pattern: "isucon9-final-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon9-final"
  - name: isucon10-qualify
    ami_pattern: "isucon10-qualify-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon10-qualify"
  - name: isucon10-final
    ami_pattern: "isucon10-final-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon10-final"
  - name: isucon11-qualify
    ami_pattern: "isucon11-qualify-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon11-qualify"
  - name: isucon11-final
    ami_pattern: "isucon11-final-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon11-final"
  - name: isucon12-qualify
    ami_pattern: "isucon12-qualify-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon12-qualify"
  - name: isucon12-final
    ami_pattern: "isucon12-final-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon12-final"
  - name: isucon13
    ami_pattern: "isucon13-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "本番は3台構成。bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon13"
  - name: isucon14
    ami_pattern: "isucon14-*"
    owner_id: "839726181030"
    ssh_user: ubuntu
    default_nodes: 1
    notes: "本番は3台構成。bench手順: https://github.com/matsuu/aws-isucon/tree/main/isucon14"
```

`internal/catalog/catalog.go`:

```go
// Package catalog はバイナリ埋め込みのISUCON過去問カタログを提供する。
package catalog

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var raw []byte

type Problem struct {
	Name         string `yaml:"name"`
	AMIPattern   string `yaml:"ami_pattern"`
	OwnerID      string `yaml:"owner_id"`
	SSHUser      string `yaml:"ssh_user"`
	DefaultNodes int    `yaml:"default_nodes"`
	Notes        string `yaml:"notes"`
}

type catalogFile struct {
	Problems []Problem `yaml:"problems"`
}

func List() []Problem {
	var f catalogFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		panic(fmt.Sprintf("embedded catalog.yaml is broken: %v", err))
	}
	return f.Problems
}

func Lookup(name string) (Problem, error) {
	for _, p := range List() {
		if p.Name == name {
			return p, nil
		}
	}
	return Problem{}, fmt.Errorf("unknown problem %q (run `isuenv problems` to list available problems)", name)
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/catalog/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/catalog/
git commit -m "feat: add embedded problem catalog with matsuu AMI patterns

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: problems コマンド

**Files:**
- Create: `cmd/problems.go`
- Test: `cmd/problems_test.go`

**Interfaces:**
- Consumes: `catalog.List()`
- Produces: `isuenv problems` サブコマンド、`renderProblems(w io.Writer)`

- [ ] **Step 1: 失敗するテストを書く**

`cmd/problems_test.go`:

```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderProblems(t *testing.T) {
	var buf bytes.Buffer
	renderProblems(&buf)
	out := buf.String()
	for _, want := range []string{"NAME", "isucon13", "isucon14", "ubuntu"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./cmd/`
Expected: FAIL（`renderProblems` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`cmd/problems.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/spf13/cobra"
)

var problemsCmd = &cobra.Command{
	Use:   "problems",
	Short: "List available ISUCON problems",
	RunE: func(cmd *cobra.Command, args []string) error {
		renderProblems(cmd.OutOrStdout())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(problemsCmd)
}

func renderProblems(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSSH USER\tNOTES")
	for _, p := range catalog.List() {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.SSHUser, p.Notes)
	}
	tw.Flush()
}
```

- [ ] **Step 4: テストと手動確認**

Run: `go test ./cmd/ && go build -o isuenv . && ./isuenv problems`
Expected: PASS、10問のテーブルが表示される

- [ ] **Step 5: コミット**

```bash
git add cmd/problems.go cmd/problems_test.go
git commit -m "feat: add problems command

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: EC2 APIインターフェースとモック（awsapi パッケージ）

**Files:**
- Create: `internal/awsapi/api.go`, `internal/awsapi/mock.go`

**Interfaces:**
- Consumes: aws-sdk-go-v2 `service/ec2`
- Produces:
  - `type EC2API interface { ... }`（下記26メソッド。`*ec2.Client` がそのまま満たす）
  - `type Mock struct { XxxFunc func(ctx, in) (out, error) }`（全メソッド分のfuncフィールド。未設定のメソッドが呼ばれたら `panic("unexpected call: Xxx")`）

- [ ] **Step 1: インターフェースを書く**

`internal/awsapi/api.go`:

```go
// Package awsapi はisuenvが使うEC2 APIの狭いインターフェースを定義する。
// *ec2.Client がこのインターフェースを満たす。テストでは Mock を使う。
package awsapi

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type EC2API interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)

	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	CreateVpc(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error)
	DeleteVpc(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error)

	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	CreateSubnet(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error)
	ModifySubnetAttribute(ctx context.Context, params *ec2.ModifySubnetAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error)
	DeleteSubnet(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error)

	DescribeInternetGateways(ctx context.Context, params *ec2.DescribeInternetGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error)
	CreateInternetGateway(ctx context.Context, params *ec2.CreateInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error)
	AttachInternetGateway(ctx context.Context, params *ec2.AttachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error)
	DetachInternetGateway(ctx context.Context, params *ec2.DetachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error)
	DeleteInternetGateway(ctx context.Context, params *ec2.DeleteInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error)

	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error)

	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngress(ctx context.Context, params *ec2.RevokeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error)
	DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)

	DescribeKeyPairs(ctx context.Context, params *ec2.DescribeKeyPairsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeKeyPairsOutput, error)
	CreateKeyPair(ctx context.Context, params *ec2.CreateKeyPairInput, optFns ...func(*ec2.Options)) (*ec2.CreateKeyPairOutput, error)
	DeleteKeyPair(ctx context.Context, params *ec2.DeleteKeyPairInput, optFns ...func(*ec2.Options)) (*ec2.DeleteKeyPairOutput, error)

	RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}
```

- [ ] **Step 2: モックを書く**

`internal/awsapi/mock.go`（パターンは全メソッド同一。**全26メソッド分**を同じ形で書くこと）:

```go
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

// ...以下、残り24メソッドもまったく同じパターンで実装する:
// CreateVpc, DeleteVpc, DescribeSubnets, CreateSubnet, ModifySubnetAttribute,
// DeleteSubnet, DescribeInternetGateways, CreateInternetGateway,
// AttachInternetGateway, DetachInternetGateway, DeleteInternetGateway,
// DescribeRouteTables, CreateRoute, DescribeSecurityGroups,
// CreateSecurityGroup, AuthorizeSecurityGroupIngress,
// RevokeSecurityGroupIngress, DeleteSecurityGroup, DescribeKeyPairs,
// CreateKeyPair, DeleteKeyPair, RunInstances, DescribeInstances,
// TerminateInstances
// （形: nilチェックでpanic("unexpected call: <名前>") → Funcフィールドに委譲）
```

- [ ] **Step 3: インターフェース適合をコンパイルで検証**

`internal/awsapi/api.go` の末尾に追記:

```go
var _ EC2API = (*ec2.Client)(nil)
```

`internal/awsapi/mock.go` の末尾に追記:

```go
var _ EC2API = (*Mock)(nil)
```

Run: `go build ./...`
Expected: 成功（`*ec2.Client` と `*Mock` の両方が `EC2API` を満たす）

- [ ] **Step 4: コミット**

```bash
git add internal/awsapi/
git commit -m "feat: add narrow EC2 API interface and hand-written mock

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Engine基盤とAMI解決

**Files:**
- Create: `internal/engine/engine.go`, `internal/engine/ami.go`
- Test: `internal/engine/ami_test.go`

**Interfaces:**
- Consumes: `awsapi.EC2API`, `catalog.Problem`
- Produces:
  - `type Engine struct { EC2 awsapi.EC2API }`
  - タグ定数 `TagManaged = "isuenv:managed"`, `TagEnv = "isuenv:env"`, `TagNode = "isuenv:node"`, `TagExpires = "isuenv:expires-at"`
  - `func (e *Engine) ResolveAMI(ctx context.Context, p catalog.Problem) (string, error)`
  - `func managedFilter() ec2types.Filter`, `func tagValue(tags []ec2types.Tag, key string) string`（パッケージ内ヘルパ）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/ami_test.go`:

```go
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
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`Engine` / `ResolveAMI` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/engine.go`:

```go
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
```

`internal/engine/ami.go`:

```go
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

// ResolveAMI は問題のAMI名パターンと所有者から最新のAMI IDを解決する。
// 古い過去問AMIはdeprecated扱いのため IncludeDeprecated が必須。
func (e *Engine) ResolveAMI(ctx context.Context, p catalog.Problem) (string, error) {
	out, err := e.EC2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners:            []string{p.OwnerID},
		IncludeDeprecated: aws.Bool(true),
		Filters: []ec2types.Filter{
			{Name: aws.String("name"), Values: []string{p.AMIPattern}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe images for %s: %w", p.Name, err)
	}
	if len(out.Images) == 0 {
		return "", fmt.Errorf("no AMI found for %s (owner %s, pattern %s)", p.Name, p.OwnerID, p.AMIPattern)
	}
	sort.Slice(out.Images, func(i, j int) bool {
		return aws.ToString(out.Images[i].CreationDate) > aws.ToString(out.Images[j].CreationDate)
	})
	return aws.ToString(out.Images[0].ImageId), nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/
git commit -m "feat: add engine base and AMI resolution with deprecated AMI support

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: TTL user-data とコスト概算（純粋関数）

**Files:**
- Create: `internal/engine/userdata.go`, `internal/engine/cost.go`
- Test: `internal/engine/userdata_test.go`, `internal/engine/cost_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `func BuildUserData(ttl time.Duration) string` — `shutdown -P +<分>` を含むシェルスクリプト（base64エンコード前の生テキスト）
  - `func HourlyUSD(instanceType string) (float64, bool)`
  - `func Estimate(since, now time.Time, hourly float64, nodes int) float64`

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/userdata_test.go`:

```go
package engine

import (
	"strings"
	"testing"
	"time"
)

func TestBuildUserData(t *testing.T) {
	ud := BuildUserData(8 * time.Hour)
	if !strings.HasPrefix(ud, "#!/bin/sh\n") {
		t.Errorf("user data must be a shell script: %q", ud)
	}
	if !strings.Contains(ud, "shutdown -P +480") {
		t.Errorf("expected shutdown after 480 minutes: %q", ud)
	}
}

func TestBuildUserData_MinimumOneMinute(t *testing.T) {
	ud := BuildUserData(10 * time.Second)
	if !strings.Contains(ud, "shutdown -P +1") {
		t.Errorf("TTL below 1 minute must clamp to 1: %q", ud)
	}
}
```

`internal/engine/cost_test.go`:

```go
package engine

import (
	"math"
	"testing"
	"time"
)

func TestHourlyUSD(t *testing.T) {
	h, ok := HourlyUSD("c5.large")
	if !ok || h <= 0 {
		t.Fatalf("c5.large must have a price: %v %v", h, ok)
	}
	if _, ok := HourlyUSD("x1e.32xlarge"); ok {
		t.Error("unknown type should return ok=false")
	}
}

func TestEstimate(t *testing.T) {
	since := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	now := since.Add(2 * time.Hour)
	got := Estimate(since, now, 0.107, 3)
	want := 2 * 0.107 * 3
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected %f, got %f", want, got)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`BuildUserData` / `HourlyUSD` / `Estimate` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/userdata.go`:

```go
package engine

import (
	"fmt"
	"time"
)

// BuildUserData はTTL経過後にインスタンス自身がシャットダウンするuser-dataを返す。
// RunInstances側で instance-initiated-shutdown-behavior=terminate と組み合わせることで
// CLIが動いていなくても環境が自己消滅する。
func BuildUserData(ttl time.Duration) string {
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("#!/bin/sh\nshutdown -P +%d \"isuenv TTL expired\"\n", minutes)
}
```

`internal/engine/cost.go`:

```go
package engine

import "time"

// ap-northeast-1のオンデマンド時間単価(USD)の概算値。2026-07時点の参考値であり課金額の保証はしない。
var hourlyUSD = map[string]float64{
	"c5.large":   0.107,
	"c5.xlarge":  0.214,
	"c5.2xlarge": 0.428,
	"c6i.large":  0.107,
	"t3.medium":  0.0544,
	"t3.large":   0.1088,
}

func HourlyUSD(instanceType string) (float64, bool) {
	h, ok := hourlyUSD[instanceType]
	return h, ok
}

func Estimate(since, now time.Time, hourly float64, nodes int) float64 {
	return now.Sub(since).Hours() * hourly * float64(nodes)
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/userdata.go internal/engine/userdata_test.go internal/engine/cost.go internal/engine/cost_test.go
git commit -m "feat: add TTL user-data builder and cost estimation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: ネットワーク（VPC/サブネット/SG）の確保とSSH元IP制限

**Files:**
- Create: `internal/engine/network.go`
- Test: `internal/engine/network_test.go`

**Interfaces:**
- Consumes: `awsapi.EC2API`, `managedFilter()`
- Produces:
  - `type Network struct { VpcID, SubnetID, SecurityGroupID string }`
  - `func (e *Engine) EnsureNetwork(ctx context.Context) (Network, error)` — isuenv管理タグ付きVPC一式を検索、なければ作成（VPC 10.100.0.0/16、パブリックサブネット 10.100.0.0/24、IGW、メインルートテーブルにデフォルトルート、SG）
  - `func (e *Engine) EnsureIngress(ctx context.Context, sgID, myIP string) error` — 既存ingressを全revokeし、`myIP/32` から tcp 22/80/443 をauthorize

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/network_test.go`:

```go
package engine

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
)

func TestEnsureNetwork_ReusesExisting(t *testing.T) {
	m := &awsapi.Mock{
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-exists")}}}, nil
		},
		DescribeSubnetsFunc: func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{Subnets: []ec2types.Subnet{{SubnetId: aws.String("subnet-exists")}}}, nil
		},
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-exists")}}}, nil
		},
	}
	e := &Engine{EC2: m}
	net, err := e.EnsureNetwork(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Network{VpcID: "vpc-exists", SubnetID: "subnet-exists", SecurityGroupID: "sg-exists"}
	if net != want {
		t.Errorf("expected %+v, got %+v", want, net)
	}
}

func TestEnsureNetwork_CreatesWhenAbsent(t *testing.T) {
	var routeIn *ec2.CreateRouteInput
	m := &awsapi.Mock{
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{}, nil
		},
		CreateVpcFunc: func(ctx context.Context, in *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
			return &ec2.CreateVpcOutput{Vpc: &ec2types.Vpc{VpcId: aws.String("vpc-new")}}, nil
		},
		CreateSubnetFunc: func(ctx context.Context, in *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
			if aws.ToString(in.VpcId) != "vpc-new" {
				t.Errorf("subnet must be created in vpc-new: %v", in.VpcId)
			}
			return &ec2.CreateSubnetOutput{Subnet: &ec2types.Subnet{SubnetId: aws.String("subnet-new")}}, nil
		},
		ModifySubnetAttributeFunc: func(ctx context.Context, in *ec2.ModifySubnetAttributeInput) (*ec2.ModifySubnetAttributeOutput, error) {
			if in.MapPublicIpOnLaunch == nil || !aws.ToBool(in.MapPublicIpOnLaunch.Value) {
				t.Error("subnet must map public IPs on launch")
			}
			return &ec2.ModifySubnetAttributeOutput{}, nil
		},
		CreateInternetGatewayFunc: func(ctx context.Context, in *ec2.CreateInternetGatewayInput) (*ec2.CreateInternetGatewayOutput, error) {
			return &ec2.CreateInternetGatewayOutput{InternetGateway: &ec2types.InternetGateway{InternetGatewayId: aws.String("igw-new")}}, nil
		},
		AttachInternetGatewayFunc: func(ctx context.Context, in *ec2.AttachInternetGatewayInput) (*ec2.AttachInternetGatewayOutput, error) {
			return &ec2.AttachInternetGatewayOutput{}, nil
		},
		DescribeRouteTablesFunc: func(ctx context.Context, in *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
			return &ec2.DescribeRouteTablesOutput{RouteTables: []ec2types.RouteTable{{RouteTableId: aws.String("rtb-main")}}}, nil
		},
		CreateRouteFunc: func(ctx context.Context, in *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
			routeIn = in
			return &ec2.CreateRouteOutput{}, nil
		},
		CreateSecurityGroupFunc: func(ctx context.Context, in *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
			return &ec2.CreateSecurityGroupOutput{GroupId: aws.String("sg-new")}, nil
		},
	}
	e := &Engine{EC2: m}
	net, err := e.EnsureNetwork(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Network{VpcID: "vpc-new", SubnetID: "subnet-new", SecurityGroupID: "sg-new"}
	if net != want {
		t.Errorf("expected %+v, got %+v", want, net)
	}
	if routeIn == nil || aws.ToString(routeIn.DestinationCidrBlock) != "0.0.0.0/0" || aws.ToString(routeIn.GatewayId) != "igw-new" {
		t.Errorf("default route to igw-new required: %+v", routeIn)
	}
}

func TestEnsureIngress_ReplacesRulesWithMyIP(t *testing.T) {
	existing := []ec2types.IpPermission{{
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(22), ToPort: aws.Int32(22),
		IpRanges: []ec2types.IpRange{{CidrIp: aws.String("203.0.113.9/32")}},
	}}
	var revoked *ec2.RevokeSecurityGroupIngressInput
	var authorized *ec2.AuthorizeSecurityGroupIngressInput
	m := &awsapi.Mock{
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1"), IpPermissions: existing}}}, nil
		},
		RevokeSecurityGroupIngressFunc: func(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
			revoked = in
			return &ec2.RevokeSecurityGroupIngressOutput{}, nil
		},
		AuthorizeSecurityGroupIngressFunc: func(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
			authorized = in
			return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	if err := e.EnsureIngress(context.Background(), "sg-1", "198.51.100.7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked == nil || len(revoked.IpPermissions) != 1 {
		t.Fatalf("existing rules must be revoked: %+v", revoked)
	}
	if authorized == nil || len(authorized.IpPermissions) != 3 {
		t.Fatalf("expected 3 rules (22/80/443): %+v", authorized)
	}
	for _, perm := range authorized.IpPermissions {
		if aws.ToString(perm.IpRanges[0].CidrIp) != "198.51.100.7/32" {
			t.Errorf("rules must be scoped to my ip: %+v", perm)
		}
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`Network` / `EnsureNetwork` / `EnsureIngress` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/network.go`:

```go
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
	if _, err := e.EC2.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       aws.String(sgID),
		IpPermissions: perms,
	}); err != nil {
		return fmt.Errorf("authorize ingress from %s: %w", cidr, err)
	}
	return nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/network.go internal/engine/network_test.go
git commit -m "feat: ensure dedicated VPC network and my-IP-only ingress

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: キーペア確保

**Files:**
- Create: `internal/engine/keypair.go`
- Test: `internal/engine/keypair_test.go`

**Interfaces:**
- Consumes: `awsapi.EC2API`
- Produces:
  - `const KeyName = "isuenv"`
  - `func (e *Engine) EnsureKeyPair(ctx context.Context, pemPath string) (string, error)` — キー名を返す。AWS側に存在しなければ作成しpemを0600で保存。AWS側に存在するがpemがローカルにない場合はエラー（復旧手順をメッセージに含める）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/keypair_test.go`:

```go
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
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`EnsureKeyPair` / `KeyName` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/keypair.go`:

```go
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
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/keypair.go internal/engine/keypair_test.go
git commit -m "feat: ensure isuenv key pair with local pem persistence

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: インスタンス起動（Up）と待機・ロールバック

**Files:**
- Create: `internal/engine/instance.go`
- Test: `internal/engine/instance_test.go`

**Interfaces:**
- Consumes: `catalog.Problem`, `BuildUserData`, `Network`, タグ定数
- Produces:
  - `var PollInterval = 5 * time.Second`（テストで短縮するためvar）
  - `type Node struct { Index int; ID, PublicIP, PrivateIP string }`
  - `type UpOptions struct { Problem catalog.Problem; AMIID string; Nodes int; InstanceType string; TTL time.Duration; KeyName string; Net Network; Now time.Time }`
  - `func (e *Engine) Up(ctx context.Context, opts UpOptions) ([]Node, error)` — 同名環境の重複チェック → ノードごとにRunInstances（タグ・user-data・terminate挙動付き）→ running+パブリックIP付与まで待機。途中失敗時は起動済みインスタンスをterminateしてロールバック
  - （このタスクでは `Up` が使う `List` の前方参照を避けるため、重複チェックは `DescribeInstances` 直呼びで実装する）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/instance_test.go`:

```go
package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

func testProblem() catalog.Problem {
	return catalog.Problem{Name: "isucon13", AMIPattern: "isucon13-*", OwnerID: "839726181030", SSHUser: "ubuntu"}
}

func runningInstance(id, env, node, ip, privIP string) ec2types.Instance {
	return ec2types.Instance{
		InstanceId:       aws.String(id),
		State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		PublicIpAddress:  aws.String(ip),
		PrivateIpAddress: aws.String(privIP),
		InstanceType:     ec2types.InstanceTypeC5Large,
		LaunchTime:       aws.Time(time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)),
		Tags: []ec2types.Tag{
			{Key: aws.String(TagManaged), Value: aws.String("true")},
			{Key: aws.String(TagEnv), Value: aws.String(env)},
			{Key: aws.String(TagNode), Value: aws.String(node)},
			{Key: aws.String(TagExpires), Value: aws.String("2026-07-08T18:00:00Z")},
		},
	}
}

func TestUp_LaunchesNodesWithTagsAndTTL(t *testing.T) {
	PollInterval = time.Millisecond
	var runs []*ec2.RunInstancesInput
	m := &awsapi.Mock{
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			runs = append(runs, in)
			id := "i-" + strconv.Itoa(len(runs))
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String(id)}}}, nil
		},
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) == 0 {
				// Upの重複チェック: 既存環境なし
				return &ec2.DescribeInstancesOutput{}, nil
			}
			// waitRunning: 全ノードrunning
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
				runningInstance("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	nodes, err := e.Up(context.Background(), UpOptions{
		Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c5.large",
		TTL: 8 * time.Hour, KeyName: "isuenv",
		Net: Network{SubnetID: "subnet-1", SecurityGroupID: "sg-1"},
		Now: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Index != 1 || nodes[1].Index != 2 {
		t.Fatalf("expected 2 sorted nodes, got %+v", nodes)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 RunInstances calls, got %d", len(runs))
	}
	first := runs[0]
	if first.InstanceInitiatedShutdownBehavior != ec2types.ShutdownBehaviorTerminate {
		t.Error("shutdown behavior must be terminate (TTL self-destruction)")
	}
	ud, err := base64.StdEncoding.DecodeString(aws.ToString(first.UserData))
	if err != nil || !strings.Contains(string(ud), "shutdown -P +480") {
		t.Errorf("user data must contain TTL shutdown: %s (err %v)", ud, err)
	}
	tags := first.TagSpecifications[0].Tags
	if tagValue(tags, TagEnv) != "isucon13" || tagValue(tags, TagNode) != "1" {
		t.Errorf("env/node tags required: %+v", tags)
	}
	if tagValue(tags, TagExpires) != "2026-07-08T18:00:00Z" {
		t.Errorf("expires tag must be Now+TTL in RFC3339 UTC: %+v", tags)
	}
}

func TestUp_DuplicateEnvRejected(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-9", "isucon13", "1", "54.0.0.9", "10.100.0.19"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.Up(context.Background(), UpOptions{Problem: testProblem(), AMIID: "ami-123", Nodes: 1, InstanceType: "c5.large", TTL: time.Hour, KeyName: "isuenv", Now: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestUp_RollbackOnLaunchFailure(t *testing.T) {
	var terminated []string
	callCount := 0
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{}, nil
		},
		RunInstancesFunc: func(ctx context.Context, in *ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
			callCount++
			if callCount == 2 {
				return nil, errors.New("InsufficientInstanceCapacity")
			}
			return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: aws.String("i-1")}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			terminated = in.InstanceIds
			return &ec2.TerminateInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	_, err := e.Up(context.Background(), UpOptions{Problem: testProblem(), AMIID: "ami-123", Nodes: 2, InstanceType: "c5.large", TTL: time.Hour, KeyName: "isuenv", Now: time.Now()})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(terminated) != 1 || terminated[0] != "i-1" {
		t.Errorf("launched instance must be rolled back: %v", terminated)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`Up` / `UpOptions` / `Node` / `PollInterval` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/instance.go`:

```go
package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/catalog"
)

// PollInterval は待機ポーリングの間隔。テストで短縮するためvarにしている。
var PollInterval = 5 * time.Second

const maxPolls = 60

type Node struct {
	Index     int
	ID        string
	PublicIP  string
	PrivateIP string
}

type UpOptions struct {
	Problem      catalog.Problem
	AMIID        string
	Nodes        int
	InstanceType string
	TTL          time.Duration
	KeyName      string
	Net          Network
	Now          time.Time
}

// Up は環境を起動し、全ノードがrunningかつパブリックIP付与済みになるまで待つ。
// 途中で失敗した場合は起動済みインスタンスをterminateしてから失敗を返す。
func (e *Engine) Up(ctx context.Context, opts UpOptions) ([]Node, error) {
	name := opts.Problem.Name
	existing, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			{Name: aws.String("tag:" + TagEnv), Values: []string{name}},
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("check existing env: %w", err)
	}
	for _, r := range existing.Reservations {
		if len(r.Instances) > 0 {
			return nil, fmt.Errorf("environment %q already exists; run `isuenv down %s` first", name, name)
		}
	}

	expires := opts.Now.Add(opts.TTL).UTC().Format(time.RFC3339)
	userData := base64.StdEncoding.EncodeToString([]byte(BuildUserData(opts.TTL)))

	var ids []string
	for i := 1; i <= opts.Nodes; i++ {
		out, err := e.EC2.RunInstances(ctx, &ec2.RunInstancesInput{
			ImageId:                           aws.String(opts.AMIID),
			InstanceType:                      ec2types.InstanceType(opts.InstanceType),
			MinCount:                          aws.Int32(1),
			MaxCount:                          aws.Int32(1),
			KeyName:                           aws.String(opts.KeyName),
			SubnetId:                          aws.String(opts.Net.SubnetID),
			SecurityGroupIds:                  []string{opts.Net.SecurityGroupID},
			UserData:                          aws.String(userData),
			InstanceInitiatedShutdownBehavior: ec2types.ShutdownBehaviorTerminate,
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String(TagManaged), Value: aws.String("true")},
					{Key: aws.String(TagEnv), Value: aws.String(name)},
					{Key: aws.String(TagNode), Value: aws.String(strconv.Itoa(i))},
					{Key: aws.String(TagExpires), Value: aws.String(expires)},
					{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("%s-%d", name, i))},
				},
			}},
		})
		if err != nil {
			e.rollback(ctx, ids)
			return nil, fmt.Errorf("launch node %d of %s: %w (launched instances were rolled back)", i, name, err)
		}
		ids = append(ids, aws.ToString(out.Instances[0].InstanceId))
	}

	nodes, err := e.waitRunning(ctx, ids)
	if err != nil {
		e.rollback(ctx, ids)
		return nil, fmt.Errorf("wait for %s: %w (instances were rolled back)", name, err)
	}
	return nodes, nil
}

func (e *Engine) waitRunning(ctx context.Context, ids []string) ([]Node, error) {
	for attempt := 0; attempt < maxPolls; attempt++ {
		out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
		if err != nil {
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		var nodes []Node
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				if inst.State == nil || inst.State.Name != ec2types.InstanceStateNameRunning || inst.PublicIpAddress == nil {
					continue
				}
				index, _ := strconv.Atoi(tagValue(inst.Tags, TagNode))
				nodes = append(nodes, Node{
					Index:     index,
					ID:        aws.ToString(inst.InstanceId),
					PublicIP:  aws.ToString(inst.PublicIpAddress),
					PrivateIP: aws.ToString(inst.PrivateIpAddress),
				})
			}
		}
		if len(nodes) == len(ids) {
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].Index < nodes[j].Index })
			return nodes, nil
		}
		time.Sleep(PollInterval)
	}
	return nil, fmt.Errorf("instances did not become running within %s", time.Duration(maxPolls)*PollInterval)
}

// rollback はUp途中失敗時の後始末。失敗してもTTLで自己消滅するためエラーは握りつぶす。
func (e *Engine) rollback(ctx context.Context, ids []string) {
	if len(ids) == 0 {
		return
	}
	_, _ = e.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids})
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/instance.go internal/engine/instance_test.go
git commit -m "feat: launch environment nodes with TTL self-destruction and rollback

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: 環境一覧（List）

**Files:**
- Modify: `internal/engine/instance.go`（末尾に追記）
- Test: `internal/engine/instance_test.go`（末尾に追記）

**Interfaces:**
- Consumes: タグ定数、`tagValue`
- Produces:
  - `type Env struct { Name, InstanceType string; LaunchedAt, ExpiresAt time.Time; Nodes []Node }`
  - `func (e *Engine) List(ctx context.Context) ([]Env, error)` — pending/runningのisuenv管理インスタンスを `isuenv:env` タグでグループ化。Envは名前順、Nodesはindex順

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/instance_test.go` に追記:

```go
func TestList_GroupsByEnv(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12"),
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
				runningInstance("i-3", "isucon14", "1", "54.0.0.3", "10.100.0.13"),
			}}}}, nil
		},
	}
	e := &Engine{EC2: m}
	envs, err := e.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d: %+v", len(envs), envs)
	}
	if envs[0].Name != "isucon13" || envs[1].Name != "isucon14" {
		t.Errorf("envs must be sorted by name: %+v", envs)
	}
	if len(envs[0].Nodes) != 2 || envs[0].Nodes[0].Index != 1 || envs[0].Nodes[1].Index != 2 {
		t.Errorf("nodes must be sorted by index: %+v", envs[0].Nodes)
	}
	wantExpires := time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)
	if !envs[0].ExpiresAt.Equal(wantExpires) {
		t.Errorf("expires-at tag must be parsed: %v", envs[0].ExpiresAt)
	}
	if envs[0].InstanceType != "c5.large" {
		t.Errorf("instance type must be captured: %v", envs[0].InstanceType)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`Env` / `List` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/instance.go` に追記:

```go
type Env struct {
	Name         string
	InstanceType string
	LaunchedAt   time.Time
	ExpiresAt    time.Time
	Nodes        []Node
}

// List はisuenv管理下の稼働中環境を isuenv:env タグでグループ化して返す。
func (e *Engine) List(ctx context.Context) ([]Env, error) {
	out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instances: %w", err)
	}
	byName := map[string]*Env{}
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			name := tagValue(inst.Tags, TagEnv)
			if name == "" {
				continue
			}
			env, ok := byName[name]
			if !ok {
				env = &Env{Name: name, InstanceType: string(inst.InstanceType)}
				if v := tagValue(inst.Tags, TagExpires); v != "" {
					if ts, err := time.Parse(time.RFC3339, v); err == nil {
						env.ExpiresAt = ts
					}
				}
				byName[name] = env
			}
			launched := aws.ToTime(inst.LaunchTime)
			if env.LaunchedAt.IsZero() || launched.Before(env.LaunchedAt) {
				env.LaunchedAt = launched
			}
			index, _ := strconv.Atoi(tagValue(inst.Tags, TagNode))
			env.Nodes = append(env.Nodes, Node{
				Index:     index,
				ID:        aws.ToString(inst.InstanceId),
				PublicIP:  aws.ToString(inst.PublicIpAddress),
				PrivateIP: aws.ToString(inst.PrivateIpAddress),
			})
		}
	}
	envs := make([]Env, 0, len(byName))
	for _, env := range byName {
		sort.Slice(env.Nodes, func(i, j int) bool { return env.Nodes[i].Index < env.Nodes[j].Index })
		envs = append(envs, *env)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs, nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/instance.go internal/engine/instance_test.go
git commit -m "feat: list running environments grouped by env tag

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: 環境削除（Down）と全消し（Nuke）

**Files:**
- Modify: `internal/engine/instance.go`（末尾に追記）
- Create: `internal/engine/nuke.go`
- Test: `internal/engine/instance_test.go`, `internal/engine/nuke_test.go`

**Interfaces:**
- Consumes: タグ定数、`Network`、`KeyName`、`PollInterval`/`maxPolls`
- Produces:
  - `func (e *Engine) Down(ctx context.Context, name string) ([]string, error)` — 対象環境のインスタンスをterminateし、対象IDを返す。対象なしなら `(nil, nil)`（冪等）
  - `func (e *Engine) Nuke(ctx context.Context) error` — 全管理インスタンスterminate→終了待ち→キーペア削除→SG/サブネット/IGW/VPC削除。存在しないリソースはスキップ（冪等）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/instance_test.go` に追記:

```go
func TestDown_TerminatesEnvInstances(t *testing.T) {
	var describeIn *ec2.DescribeInstancesInput
	var terminated []string
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			describeIn = in
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
				runningInstance("i-2", "isucon13", "2", "54.0.0.2", "10.100.0.12"),
			}}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			terminated = in.InstanceIds
			return &ec2.TerminateInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	ids, err := e.Down(context.Background(), "isucon13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || len(terminated) != 2 {
		t.Errorf("expected 2 instances terminated: ids=%v terminated=%v", ids, terminated)
	}
	found := false
	for _, f := range describeIn.Filters {
		if aws.ToString(f.Name) == "tag:"+TagEnv && f.Values[0] == "isucon13" {
			found = true
		}
	}
	if !found {
		t.Errorf("describe must filter by env tag: %+v", describeIn.Filters)
	}
}

func TestDown_NoInstancesIsNoop(t *testing.T) {
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	ids, err := e.Down(context.Background(), "isucon13")
	if err != nil {
		t.Fatalf("down must be idempotent: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected no ids, got %v", ids)
	}
}
```

`internal/engine/nuke_test.go`:

```go
package engine

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/kyosu-1/isuenv/internal/awsapi"
)

func TestNuke_DeletesEverything(t *testing.T) {
	PollInterval = time.Millisecond
	deleted := map[string]bool{}
	m := &awsapi.Mock{
		DescribeInstancesFunc: func(ctx context.Context, in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
			if len(in.InstanceIds) > 0 {
				// 終了待ちポーリング: terminated扱い
				return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
					InstanceId: aws.String("i-1"),
					State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameTerminated},
				}}}}}, nil
			}
			return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				runningInstance("i-1", "isucon13", "1", "54.0.0.1", "10.100.0.11"),
			}}}}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
			deleted["instances"] = true
			return &ec2.TerminateInstancesOutput{}, nil
		},
		DeleteKeyPairFunc: func(ctx context.Context, in *ec2.DeleteKeyPairInput) (*ec2.DeleteKeyPairOutput, error) {
			deleted["keypair"] = true
			return &ec2.DeleteKeyPairOutput{}, nil
		},
		DescribeVpcsFunc: func(ctx context.Context, in *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-1")}}}, nil
		},
		DescribeSecurityGroupsFunc: func(ctx context.Context, in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1")}}}, nil
		},
		DeleteSecurityGroupFunc: func(ctx context.Context, in *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
			deleted["sg"] = true
			return &ec2.DeleteSecurityGroupOutput{}, nil
		},
		DescribeSubnetsFunc: func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{Subnets: []ec2types.Subnet{{SubnetId: aws.String("subnet-1")}}}, nil
		},
		DeleteSubnetFunc: func(ctx context.Context, in *ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error) {
			deleted["subnet"] = true
			return &ec2.DeleteSubnetOutput{}, nil
		},
		DescribeInternetGatewaysFunc: func(ctx context.Context, in *ec2.DescribeInternetGatewaysInput) (*ec2.DescribeInternetGatewaysOutput, error) {
			return &ec2.DescribeInternetGatewaysOutput{InternetGateways: []ec2types.InternetGateway{{InternetGatewayId: aws.String("igw-1")}}}, nil
		},
		DetachInternetGatewayFunc: func(ctx context.Context, in *ec2.DetachInternetGatewayInput) (*ec2.DetachInternetGatewayOutput, error) {
			deleted["igw-detach"] = true
			return &ec2.DetachInternetGatewayOutput{}, nil
		},
		DeleteInternetGatewayFunc: func(ctx context.Context, in *ec2.DeleteInternetGatewayInput) (*ec2.DeleteInternetGatewayOutput, error) {
			deleted["igw"] = true
			return &ec2.DeleteInternetGatewayOutput{}, nil
		},
		DeleteVpcFunc: func(ctx context.Context, in *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error) {
			deleted["vpc"] = true
			return &ec2.DeleteVpcOutput{}, nil
		},
	}
	e := &Engine{EC2: m}
	if err := e.Nuke(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"instances", "keypair", "sg", "subnet", "igw-detach", "igw", "vpc"} {
		if !deleted[key] {
			t.Errorf("nuke must delete %s", key)
		}
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/`
Expected: FAIL（`Down` / `Nuke` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/engine/instance.go` に追記:

```go
// Down は環境のインスタンスをterminateし、対象のインスタンスIDを返す。対象なしは成功扱い。
func (e *Engine) Down(ctx context.Context, name string) ([]string, error) {
	out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			managedFilter(),
			{Name: aws.String("tag:" + TagEnv), Values: []string{name}},
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instances for %s: %w", name, err)
	}
	var ids []string
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			ids = append(ids, aws.ToString(inst.InstanceId))
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := e.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids}); err != nil {
		return nil, fmt.Errorf("terminate %v: %w", ids, err)
	}
	return ids, nil
}

func (e *Engine) waitTerminated(ctx context.Context, ids []string) error {
	for attempt := 0; attempt < maxPolls; attempt++ {
		out, err := e.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
		if err != nil {
			return fmt.Errorf("describe instances: %w", err)
		}
		done := true
		for _, r := range out.Reservations {
			for _, inst := range r.Instances {
				if inst.State == nil || inst.State.Name != ec2types.InstanceStateNameTerminated {
					done = false
				}
			}
		}
		if done {
			return nil
		}
		time.Sleep(PollInterval)
	}
	return fmt.Errorf("instances did not terminate within %s", time.Duration(maxPolls)*PollInterval)
}
```

`internal/engine/nuke.go`:

```go
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
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
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

	// キーペアは存在しなくてもエラーにしない（冪等性のため無視）
	_, _ = e.EC2.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(KeyName)})

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
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/engine/instance.go internal/engine/instance_test.go internal/engine/nuke.go internal/engine/nuke_test.go
git commit -m "feat: add idempotent down and full-cleanup nuke

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: SSH config 生成（sshconf パッケージ）

**Files:**
- Create: `internal/sshconf/sshconf.go`
- Test: `internal/sshconf/sshconf_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `type Host struct { Alias, HostName, User, IdentityFile string }`
  - `func Render(hosts []Host) string`
  - `func WriteConfig(path string, hosts []Host) error`（0600で書き込み）
  - `func EnsureInclude(configPath, includePath string) error` — `Include <includePath>` 行がなければ先頭に追記（冪等）。configPathが存在しなければ新規作成

- [ ] **Step 1: 失敗するテストを書く**

`internal/sshconf/sshconf_test.go`:

```go
package sshconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	got := Render([]Host{
		{Alias: "isucon13-1", HostName: "54.0.0.1", User: "ubuntu", IdentityFile: "/home/u/.ssh/isuenv.pem"},
	})
	for _, want := range []string{
		"Host isucon13-1",
		"HostName 54.0.0.1",
		"User ubuntu",
		"IdentityFile /home/u/.ssh/isuenv.pem",
		"StrictHostKeyChecking no",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered config should contain %q:\n%s", want, got)
		}
	}
}

func TestEnsureInclude_Idempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("Host example\n  HostName example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := EnsureInclude(configPath, "~/.ssh/isuenv_config"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	data, _ := os.ReadFile(configPath)
	if got := strings.Count(string(data), "Include ~/.ssh/isuenv_config"); got != 1 {
		t.Errorf("Include line must appear exactly once, got %d:\n%s", got, data)
	}
	if !strings.Contains(string(data), "Host example") {
		t.Error("existing content must be preserved")
	}
}

func TestEnsureInclude_CreatesConfigIfMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config")
	if err := EnsureInclude(configPath, "~/.ssh/isuenv_config"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config must be created: %v", err)
	}
	if !strings.Contains(string(data), "Include ~/.ssh/isuenv_config") {
		t.Errorf("include line missing:\n%s", data)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/sshconf/`
Expected: FAIL（未定義シンボルのコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/sshconf/sshconf.go`:

```go
// Package sshconf はisuenv環境向けのssh config生成を提供する。
package sshconf

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Host struct {
	Alias        string
	HostName     string
	User         string
	IdentityFile string
}

// Render は練習環境用のssh config断片を生成する。
// 環境は使い捨てでIPが再利用されるためホストキー検証は無効化する。
func Render(hosts []Host) string {
	var b strings.Builder
	b.WriteString("# Generated by isuenv. DO NOT EDIT.\n")
	for _, h := range hosts {
		fmt.Fprintf(&b, "\nHost %s\n  HostName %s\n  User %s\n  IdentityFile %s\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n",
			h.Alias, h.HostName, h.User, h.IdentityFile)
	}
	return b.String()
}

func WriteConfig(path string, hosts []Host) error {
	return os.WriteFile(path, []byte(Render(hosts)), 0o600)
}

// EnsureInclude は configPath の先頭に Include 行を一度だけ追加する。
func EnsureInclude(configPath, includePath string) error {
	line := "Include " + includePath
	data, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if strings.Contains(string(data), line) {
		return nil
	}
	content := line + "\n" + string(data)
	return os.WriteFile(configPath, []byte(content), 0o600)
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/sshconf/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/sshconf/
git commit -m "feat: add ssh config generation with idempotent include

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: グローバルIP取得（myip パッケージ）

**Files:**
- Create: `internal/myip/myip.go`
- Test: `internal/myip/myip_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `var Endpoint = "https://checkip.amazonaws.com"`（テストで差し替えるためvar）
  - `func Get(ctx context.Context) (string, error)` — 応答をtrimしてIPとして検証した文字列を返す

- [ ] **Step 1: 失敗するテストを書く**

`internal/myip/myip_test.go`:

```go
package myip

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "198.51.100.7")
	}))
	defer srv.Close()
	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	ip, err := Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "198.51.100.7" {
		t.Errorf("expected trimmed ip, got %q", ip)
	}
}

func TestGet_InvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "<html>not an ip</html>")
	}))
	defer srv.Close()
	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	if _, err := Get(context.Background()); err == nil {
		t.Fatal("expected error for non-IP response")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/myip/`
Expected: FAIL（未定義シンボルのコンパイルエラー）

- [ ] **Step 3: 実装を書く**

`internal/myip/myip.go`:

```go
// Package myip は実行元のグローバルIPアドレスを取得する。
package myip

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Endpoint はグローバルIP取得先。テストで差し替えるためvarにしている。
var Endpoint = "https://checkip.amazonaws.com"

func Get(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get global ip from %s: %w", Endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get global ip: unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("get global ip: invalid response %q", ip)
	}
	return ip, nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/myip/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/myip/
git commit -m "feat: add global IP lookup

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 14: コマンド配線（up / list / ssh / down / nuke）

**Files:**
- Create: `cmd/sshsync.go`, `cmd/up.go`, `cmd/list.go`, `cmd/ssh.go`, `cmd/down.go`, `cmd/nuke.go`

**Interfaces:**
- Consumes: `engine.*`, `catalog.*`, `sshconf.*`, `myip.Get`, `newEC2Client` / `pemPath` / `sshDir`（Task 1）
- Produces: `isuenv up/list/ssh/down/nuke` サブコマンド、`refreshSSHConfig(ctx, e) error`

このタスクは薄いglue層のためユニットテストは書かない（ロジックは全てテスト済みのinternalパッケージにある）。`go build` / `go vet` と、AWS認証なしでのエラーメッセージ確認で検証する。

- [ ] **Step 1: cmd/sshsync.go を書く**

```go
package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/kyosu-1/isuenv/internal/sshconf"
)

// refreshSSHConfig は稼働中環境から ~/.ssh/isuenv_config を再生成し、
// ~/.ssh/config へのInclude行を保証する。
func refreshSSHConfig(ctx context.Context, e *engine.Engine) error {
	envs, err := e.List(ctx)
	if err != nil {
		return err
	}
	var hosts []sshconf.Host
	for _, env := range envs {
		user := "ubuntu"
		if p, err := catalog.Lookup(env.Name); err == nil {
			user = p.SSHUser
		}
		for _, n := range env.Nodes {
			hosts = append(hosts, sshconf.Host{
				Alias:        fmt.Sprintf("%s-%d", env.Name, n.Index),
				HostName:     n.PublicIP,
				User:         user,
				IdentityFile: pemPath(),
			})
		}
	}
	includeFile := filepath.Join(sshDir(), "isuenv_config")
	if err := sshconf.WriteConfig(includeFile, hosts); err != nil {
		return err
	}
	return sshconf.EnsureInclude(filepath.Join(sshDir(), "config"), includeFile)
}
```

- [ ] **Step 2: cmd/up.go を書く**

```go
package cmd

import (
	"fmt"
	"time"

	"github.com/kyosu-1/isuenv/internal/catalog"
	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/kyosu-1/isuenv/internal/myip"
	"github.com/spf13/cobra"
)

var (
	upNodes        int
	upTTL          time.Duration
	upInstanceType string
)

var upCmd = &cobra.Command{
	Use:   "up <problem>",
	Short: "Create a practice environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		p, err := catalog.Lookup(args[0])
		if err != nil {
			return err
		}
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}

		fmt.Printf("Resolving AMI for %s...\n", p.Name)
		ami, err := e.ResolveAMI(ctx, p)
		if err != nil {
			return err
		}
		fmt.Println("Ensuring network...")
		net, err := e.EnsureNetwork(ctx)
		if err != nil {
			return err
		}
		ip, err := myip.Get(ctx)
		if err != nil {
			return err
		}
		if err := e.EnsureIngress(ctx, net.SecurityGroupID, ip); err != nil {
			return err
		}
		key, err := e.EnsureKeyPair(ctx, pemPath())
		if err != nil {
			return err
		}
		fmt.Printf("Launching %d node(s) of %s (%s, TTL %s)...\n", upNodes, p.Name, upInstanceType, upTTL)
		nodes, err := e.Up(ctx, engine.UpOptions{
			Problem: p, AMIID: ami, Nodes: upNodes, InstanceType: upInstanceType,
			TTL: upTTL, KeyName: key, Net: net, Now: time.Now(),
		})
		if err != nil {
			return err
		}
		if err := refreshSSHConfig(ctx, e); err != nil {
			return err
		}
		fmt.Printf("\n%s is ready. Auto-terminates in %s.\n\n", p.Name, upTTL)
		for _, n := range nodes {
			fmt.Printf("  %s-%d  public %s  private %s  (ssh %s-%d)\n", p.Name, n.Index, n.PublicIP, n.PrivateIP, p.Name, n.Index)
		}
		if p.Notes != "" {
			fmt.Printf("\nNote: %s\n", p.Notes)
		}
		return nil
	},
}

func init() {
	upCmd.Flags().IntVar(&upNodes, "nodes", 1, "number of nodes to launch")
	upCmd.Flags().DurationVar(&upTTL, "ttl", 8*time.Hour, "auto-terminate after this duration")
	upCmd.Flags().StringVar(&upInstanceType, "instance-type", "c5.large", "EC2 instance type")
	rootCmd.AddCommand(upCmd)
}
```

- [ ] **Step 3: cmd/list.go を書く**

```go
package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running practice environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		envs, err := e.List(ctx)
		if err != nil {
			return err
		}
		if len(envs) == 0 {
			fmt.Println("No running environments.")
			return nil
		}
		now := time.Now()
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ENV\tNODES\tTYPE\tUPTIME\tEST COST\tTTL LEFT\tPUBLIC IPS")
		for _, env := range envs {
			cost := "-"
			if h, ok := engine.HourlyUSD(env.InstanceType); ok {
				cost = fmt.Sprintf("$%.2f", engine.Estimate(env.LaunchedAt, now, h, len(env.Nodes)))
			}
			ttlLeft := "-"
			if !env.ExpiresAt.IsZero() {
				ttlLeft = time.Until(env.ExpiresAt).Round(time.Minute).String()
			}
			var ips []string
			for _, n := range env.Nodes {
				ips = append(ips, n.PublicIP)
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				env.Name, len(env.Nodes), env.InstanceType,
				now.Sub(env.LaunchedAt).Round(time.Minute), cost, ttlLeft, strings.Join(ips, ", "))
		}
		return tw.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
```

- [ ] **Step 4: cmd/ssh.go を書く**

```go
package cmd

import (
	"os"
	"os/exec"
	"regexp"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var nodeSuffix = regexp.MustCompile(`-\d+$`)

var sshCmd = &cobra.Command{
	Use:   "ssh <problem>[-N]",
	Short: "SSH into a practice environment node (node 1 by default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		if err := refreshSSHConfig(ctx, e); err != nil {
			return err
		}
		alias := args[0]
		if !nodeSuffix.MatchString(alias) {
			alias += "-1"
		}
		ssh := exec.CommandContext(ctx, "ssh", alias)
		ssh.Stdin = os.Stdin
		ssh.Stdout = os.Stdout
		ssh.Stderr = os.Stderr
		return ssh.Run()
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
```

- [ ] **Step 5: cmd/down.go と cmd/nuke.go を書く**

`cmd/down.go`:

```go
package cmd

import (
	"fmt"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down <problem>",
	Short: "Terminate a practice environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		ids, err := e.Down(ctx, args[0])
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			fmt.Printf("No running environment %q. Nothing to do.\n", args[0])
			return nil
		}
		fmt.Printf("Terminating %s: %v\n", args[0], ids)
		return refreshSSHConfig(ctx, e)
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
```

`cmd/nuke.go`:

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kyosu-1/isuenv/internal/engine"
	"github.com/spf13/cobra"
)

var nukeCmd = &cobra.Command{
	Use:   "nuke",
	Short: "Delete ALL isuenv-managed resources (instances, key pair, VPC)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("This deletes ALL isuenv resources on AWS. Type 'yes' to continue: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(answer) != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
		ctx := cmd.Context()
		client, err := newEC2Client(ctx)
		if err != nil {
			return err
		}
		e := &engine.Engine{EC2: client}
		if err := e.Nuke(ctx); err != nil {
			return err
		}
		fmt.Println("All isuenv resources deleted.")
		return refreshSSHConfig(ctx, e)
	},
}

func init() {
	rootCmd.AddCommand(nukeCmd)
}
```

- [ ] **Step 6: ビルド・vet・全テスト・スモーク確認**

Run: `go build -o isuenv . && go vet ./... && go test ./...`
Expected: すべて成功

Run: `./isuenv --help`
Expected: `up` / `down` / `list` / `ssh` / `nuke` / `problems` が表示される

Run: `AWS_ACCESS_KEY_ID= AWS_SECRET_ACCESS_KEY= AWS_PROFILE=nonexistent ./isuenv list; echo "exit=$?"`
Expected: 認証エラーのメッセージが表示され exit=1（パニックしない）

- [ ] **Step 7: コミット**

```bash
git add cmd/
git commit -m "feat: wire up/list/ssh/down/nuke commands

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 15: README と手動E2E手順

**Files:**
- Create: `README.md`

**Interfaces:**
- Consumes: 全タスクの成果物
- Produces: 利用手順・手動E2E検証手順のドキュメント

- [ ] **Step 1: README.md を書く**

以下の内容を含める（文面は整えてよいが項目は必須）:

````markdown
# isuenv

ISUCON過去問の練習環境（[matsuu/aws-isucon](https://github.com/matsuu/aws-isucon) の公開AMI）を
AWS EC2上にコマンド一発で構築・破棄するCLI。

## インストール

```sh
go install github.com/kyosu-1/isuenv@latest
```

## 前提

- AWS認証情報（`AWS_PROFILE` などSDKの標準的な方法で解決される）
- リージョンは ap-northeast-1 固定
- EC2の vCPU クォータ（複数台構成を使う場合は6 vCPU以上）

## 使い方

```sh
isuenv problems               # 対応過去問一覧
isuenv up isucon13            # 環境作成（1台, TTL 8h, c5.large）
isuenv up isucon13 --nodes 3  # 本番同様の3台構成
isuenv list                   # 稼働中環境と概算コスト・残りTTL
isuenv ssh isucon13           # 1号機にSSH（isucon13-2 で2号機）
isuenv down isucon13          # 環境削除
isuenv nuke                   # isuenv管理の全リソース削除（VPC・キーペア含む）
```

- 環境はTTL経過後に**自動でterminate**される（`shutdown -P` + terminate挙動）
- SSHは `~/.ssh/isuenv_config` に自動生成される。素の `ssh isucon13-1` やVS Code Remoteも使える
- セキュリティグループは実行時のグローバルIPのみ許可。IPが変わったら `isuenv ssh` を打ち直せば更新される

## コストの目安

c5.large（デフォルト）は約$0.107/時（ap-northeast-1、2026-07時点の概算）。
`isuenv list` の EST COST は概算であり、実際の課金はAWSの請求を確認すること。

## 手動E2E検証手順

コードを変更したら以下を実施する:

1. `go test ./... && go build -o isuenv .`
2. `./isuenv up isucon14 --ttl 1h`
3. `./isuenv list` — 環境が表示され、TTL LEFTが1h弱であること
4. `./isuenv ssh isucon14` — ログインできること。`sudo -i -u isucon` でアプリを確認
5. ブラウザで `http://<public ip>` にアクセスできること
6. `./isuenv down isucon14` — 削除されること
7. `./isuenv list` — 空になること
8. （まれに）`./isuenv nuke` でVPCまで消えることをAWSコンソールで確認

## 注意

- ベンチマーカーはAMIに同梱されている。実行方法は問題ごとに異なるので `isuenv problems` のNOTESのリンク先を参照
- 消し忘れてもTTLで自己消滅するが、`isuenv list` での確認を習慣にすること
````

- [ ] **Step 2: 最終確認とコミット**

Run: `go test ./... && go build -o isuenv . && ./isuenv problems`
Expected: すべて成功

```bash
git add README.md
git commit -m "docs: add README with usage and manual E2E procedure

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
