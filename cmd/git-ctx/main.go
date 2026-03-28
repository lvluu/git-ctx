package main

import (
	"fmt"
	"log"
	"os"

	"github.com/lvluu/git-ctx/internal/app"
	"github.com/lvluu/git-ctx/internal/config"
	"github.com/lvluu/git-ctx/internal/git"
	"github.com/lvluu/git-ctx/internal/profile"
	"github.com/spf13/cobra"
)

func main() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Printf("Warning: failed to load config: %v\n", err)
		// Use defaults - allow init/shell-init to work even with broken config
	}
	g := git.ExecRunner{}
	mgr := profile.NewManager(cfg.ProfilesPath)

	rootCmd := &cobra.Command{
		Use:     "git-ctx",
		Short:   "Manage git identity and worktree context",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", app.Version, app.Commit, app.Date),
	}
	rootCmd.SetVersionTemplate("git-ctx\nVersion: {{.Version}}")

	var initForce bool
	initCmd := &cobra.Command{
		Use: "init",
		Run: func(cmd *cobra.Command, args []string) {
			path := config.DefaultPath()
			if err := config.Init(path, initForce); err != nil {
				fmt.Println("init failed:", err)
				os.Exit(1)
			}
			fmt.Println("Config initialized:", path)
		},
	}
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config")

	shellInitCmd := &cobra.Command{
		Use: "shell-init",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(app.ShellInitScript())
		},
	}

	rootCmd.AddCommand(
		app.BuildProfileCmd(mgr, g, cfg),
		app.BuildWorktreeCmd(cfg, g),
		initCmd,
		shellInitCmd,
		app.BuildDoctorCmd(cfg, mgr, g),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
