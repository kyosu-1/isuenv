package cmd

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// リリースビルド時に goreleaser の ldflags で上書きされる。
// 素の go build ではデフォルト値のままになる。
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the isuenv version",
	RunE: func(cmd *cobra.Command, args []string) error {
		renderVersion(cmd.OutOrStdout())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Version = versionString()
}

func versionString() string {
	return fmt.Sprintf("%s (%s, %s)", resolveVersion(), commit, date)
}

// resolveVersion は ldflags で注入された値を優先し、無ければモジュールのビルド情報を見る。
// READMEが案内している `go install ...@latest` は ldflags を通らないため、
// これが無いとインストール経路によって version が常に "dev" になる。
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return version
}

func renderVersion(w io.Writer) {
	fmt.Fprintf(w, "isuenv %s\n", versionString())
}
