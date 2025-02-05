package build

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/ctbur/ci-server/v2/internal/disk"
)

type BuildCmd struct {
	BuildImage string
	Cmd        []string
}

type Builder struct {
	disk *disk.Disk
}

func NewBuilder(dd *disk.Disk) Builder {
	return Builder{dd}
}

func debugCmd(cmd *exec.Cmd) {
	fmt.Println("Running command:", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func (b *Builder) Build(owner, name, cloneURL, commitSHA string, cmd BuildCmd) error {
	repoDir, err := b.disk.GetRepoDir(owner, name)
	if err != nil {
		return fmt.Errorf("failed to obtain repo dir: %w", err)
	}

	initCmd := exec.Command("git", "-C", repoDir, "init", "-q")
	debugCmd(initCmd)
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to init repo at '%s': %v", repoDir, err)
	}

	cloneCmd := exec.Command("git", "-C", repoDir, "fetch", "--depth=1", cloneURL, commitSHA)
	debugCmd(cloneCmd)
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch repo at '%s': %v", cloneURL, err)
	}

	buildDir, err := b.disk.GetBuildDir(owner, name)

	// checkout repo files to build dir
	checkoutCmd := exec.Command(
		"git",
		"--git-dir", fmt.Sprintf("%s/.git", repoDir),
		"--work-tree", buildDir,
		"checkout", commitSHA, "--", ".",
	)
	debugCmd(checkoutCmd)
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit for '%s': %v", cloneURL, err)
	}

	dockerCmdArgs := []string{
		"docker", "run",
		"--volume", fmt.Sprintf("%s:/build", buildDir),
		"--workdir", "/build",
		cmd.BuildImage,
	}
	dockerCmdArgs = append(dockerCmdArgs, cmd.Cmd...)

	buildCmd := exec.Command(dockerCmdArgs[0], dockerCmdArgs[1:]...)
	debugCmd(buildCmd)
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build repository")
	}

	return nil
}
