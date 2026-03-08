package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	appCfg, err := loadAppConfig(defaultAppConfigPath())
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	git := ExecGitRunner{}
	cm := NewConfigManager(appCfg.ProfilesPath)

	rootCmd := &cobra.Command{
		Use:     "git-ctx",
		Short:   "🦑 Manage git identity and worktree context",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}
	rootCmd.SetVersionTemplate("🦑 git-ctx\nVersion: {{.Version}}")

	// init
	var initForce bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize git-ctx config file (~/.git-ctx.yaml)",
		Run: func(cmd *cobra.Command, args []string) {
			cfgPath := defaultAppConfigPath()
			if err := initAppConfig(cfgPath, initForce); err != nil {
				fmt.Println("init failed:", err)
				os.Exit(1)
			}
			fmt.Println("Config initialized:", cfgPath)
		},
	}
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config")

	// shell-init
	shellInitCmd := &cobra.Command{
		Use:   "shell-init",
		Short: "Print shell integration snippet (eval in ~/.bashrc or ~/.zshrc)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(shellInitScript())
		},
	}

	rootCmd.AddCommand(
		buildProfileCmd(cm, git, appCfg),
		buildWorktreeCmd(appCfg, git),
		initCmd,
		shellInitCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
