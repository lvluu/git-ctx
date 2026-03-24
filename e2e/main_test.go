package e2e

import (
	"os"
	"os/exec"
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
	cmd := exec.Command("go", "build", "-o", "git-ctx-test", ".")
	cmd.Dir = "/home/lvluu/git-profile"
	return cmd.Run()
}
