package build

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/ctbur/ci-server/v2/internal/store"
)

func debugCmd(cmd *exec.Cmd) {
	fmt.Println("Running command:", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func checkout(repo *store.RepoMeta, commitSHA, targetDir string) error {
	initCmd := exec.Command("git", "-C", targetDir, "init", "-q")
	debugCmd(initCmd)
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to init repo at '%s': %v", targetDir, err)
	}

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", repo.Owner, repo.Name)
	cloneCmd := exec.Command("git", "-C", targetDir, "fetch", "--depth=1", cloneURL, commitSHA)
	debugCmd(cloneCmd)
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch repo at '%s': %v", cloneURL, err)
	}

	// Checkout repo files to build dir
	checkoutCmd := exec.Command(
		"git",
		"--git-dir", fmt.Sprintf("%s/.git", targetDir),
		"--work-tree", targetDir,
		"checkout", commitSHA, "--", ".",
	)
	debugCmd(checkoutCmd)
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit for '%s': %v", cloneURL, err)
	}

	return nil
}
