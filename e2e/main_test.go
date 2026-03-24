package e2e

import (
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
	wd, _ := os.Getwd()
	// Build from project root with explicit path to cmd/git-ctx
	cmd := exec.Command("go", "build", "-o", filepath.Join(wd, "git-ctx-test"), "./cmd/git-ctx")
	cmd.Dir = filepath.Join(wd, "..")
	return cmd.Run()
}

func testBinaryPath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "git-ctx-test")
}
