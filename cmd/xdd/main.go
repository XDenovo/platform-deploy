package main

import (
	"os"

	"github.com/XDenovo/platform-deploy/internal/cli"
	"github.com/spf13/cobra"
)

func main() {
	repoRoot, err := os.Getwd()
	cobra.CheckErr(err)

	command := cli.New(cli.Options{
		RepoRoot:    repoRoot,
		ProjectName: os.Getenv("XDD_LOCAL_PROJECT_NAME"),
		EnvFile:     os.Getenv("XDD_LOCAL_ENV_FILE"),
		Run:         cli.Run,
	})
	cobra.CheckErr(command.Execute())
}
