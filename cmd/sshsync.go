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
// excludeIDs には「たった今terminateしたインスタンスID」を渡す。DescribeInstancesは結果整合性のため
// terminate直後でも対象をrunningとして返すことがあり、除外しないと消したホストが書き戻されてしまう。
func refreshSSHConfig(ctx context.Context, e *engine.Engine, excludeIDs ...string) error {
	envs, err := e.List(ctx)
	if err != nil {
		return err
	}
	excluded := make(map[string]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = true
	}
	var hosts []sshconf.Host
	for _, env := range envs {
		user := "ubuntu"
		if p, err := catalog.Lookup(env.Name); err == nil {
			user = p.SSHUser
		}
		for _, n := range env.Nodes {
			if excluded[n.ID] {
				continue
			}
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
