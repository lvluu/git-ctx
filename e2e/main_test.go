package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("E2E_TEST") == "" {
		os.Setenv("E2E_TEST", "1")
	}

	if err := rebuildBinary(); err != nil {
		os.Stderr.WriteString("Failed to rebuild binary: " + err.Error() + "\n")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func rebuildBinary() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	// Build from project root with explicit path to cmd/git-ctx
	cmd := exec.Command("go", "build", "-o", filepath.Join(wd, "git-ctx-test"), "./cmd/git-ctx")
	cmd.Dir = filepath.Join(wd, "..")
	return cmd.Run()
}

func testBinaryPath() string {
	wd, err := os.Getwd()
	if err != nil {
		panic("failed to get working directory: " + err.Error())
	}
	return filepath.Join(wd, "git-ctx-test")
}
