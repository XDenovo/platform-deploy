package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	localProjectName = "xdenovo-platform-local"
	smokeProjectName = "xdenovo-platform-local-smoke"
)

type Command struct {
	Name   string
	Args   []string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Options struct {
	RepoRoot    string
	ProjectName string
	EnvFile     string
	Run         func(context.Context, Command) error
}

func New(options Options) *cobra.Command {
	root := &cobra.Command{
		Use:           "xdd",
		Short:         "Operate the XDenovo Local deployment",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	localCommand := &cobra.Command{
		Use:   "local",
		Short: "Manage the Local environment",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	localCommand.AddCommand(
		newProfileCommand(options, "dev", []string{"dev"}),
		newProfileCommand(options, "full", []string{"dev", "full"}),
	)
	root.AddCommand(localCommand)
	return root
}

func newProfileCommand(options Options, name string, profiles []string) *cobra.Command {
	profileCommand := &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Manage the Local %s profile", name),
		Args: func(command *cobra.Command, args []string) error {
			if command.ArgsLenAtDash() != 0 || len(args) == 0 {
				return fmt.Errorf("pass Docker Compose arguments after --")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			if err := validatePassThrough(args); err != nil {
				return err
			}
			return runCompose(options, command, profiles, args...)
		},
	}
	checkCommand := &cobra.Command{
		Use:   "check",
		Short: "Validate the rendered Compose configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runCompose(options, command, profiles, "config", "--quiet"); err != nil {
				return err
			}
			_, err := fmt.Fprintf(command.OutOrStdout(), "Local %s Compose configuration is valid.\n", name)
			return err
		},
	}

	profileCommand.AddCommand(
		checkCommand,
		newUpCommand(options, name, profiles),
		newComposeAction(options, profiles, "down", "Stop services while preserving named volumes", "down", "--remove-orphans"),
		newInitCommand(options),
		newBootstrapCommand(options, profiles),
		newResetCommand(options, profiles),
	)
	return profileCommand
}

func newUpCommand(options Options, profileName string, profiles []string) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start services and wait for readiness",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if profileName == "full" {
				if err := preflightFullSources(options.RepoRoot); err != nil {
					return err
				}
			}
			return runCompose(options, command, profiles, "up", "-d", "--wait")
		},
	}
}

func newComposeAction(options Options, profiles []string, name, short string, action ...string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runCompose(options, command, profiles, action...)
		},
	}
}

func newInitCommand(options Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create config/local.env with random Local secrets",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			envFile := localEnvPath(options)
			if err := createLocalEnv(envFile); err != nil {
				return err
			}
			_, err := fmt.Fprintf(
				command.OutOrStdout(),
				"Created Local environment file %s with mode 0600.\n",
				envFile,
			)
			return err
		},
	}
}

func newBootstrapCommand(options Options, profiles []string) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Converge deployment-owned Local resources",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runCompose(options, command, profiles, "config", "--quiet"); err != nil {
				return err
			}
			if err := runComposeWithOutput(
				options,
				command,
				profiles,
				io.Discard,
				io.Discard,
				"exec", "--no-TTY", "postgres",
				"pg_isready", "--username", "xdenovo_bootstrap", "--dbname", "postgres",
			); err != nil {
				return fmt.Errorf("Local PostgreSQL is not ready; run up and inspect ps or logs: %w", err)
			}
			if err := runCompose(
				options,
				command,
				profiles,
				"exec", "--no-TTY", "postgres",
				"psql",
				"--username", "xdenovo_bootstrap",
				"--dbname", "postgres",
				"--set", "ON_ERROR_STOP=1",
				"--file", "/opt/xdenovo/bootstrap-platform.sql",
			); err != nil {
				return err
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "Local PostgreSQL bootstrap is complete.")
			return err
		},
	}
}

func newResetCommand(options Options, profiles []string) *cobra.Command {
	var confirmDestroyData bool
	command := &cobra.Command{
		Use:   "reset",
		Short: "Delete Local named volumes",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !confirmDestroyData {
				return fmt.Errorf("reset destroys Local data and requires --confirm-destroy-data")
			}
			if err := runCompose(options, command, profiles, "config", "--quiet"); err != nil {
				return err
			}
			volumeNames, err := resolveNamedVolumes(options, command)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(
				command.OutOrStdout(),
				"Destructive reset targets:\n"+
					"  Compose project: %s\n"+
					"  Named volumes:\n",
				options.projectName(),
			); err != nil {
				return err
			}
			if len(volumeNames) == 0 {
				if _, err = fmt.Fprintln(command.OutOrStdout(), "    (none)"); err != nil {
					return err
				}
			}
			for _, volumeName := range volumeNames {
				if _, err = fmt.Fprintf(command.OutOrStdout(), "    %s\n", volumeName); err != nil {
					return err
				}
			}
			if err = runCompose(options, command, profiles, "down", "--remove-orphans"); err != nil {
				return err
			}
			if len(volumeNames) == 0 {
				return nil
			}
			return runDockerWithOutput(
				options,
				command,
				command.OutOrStdout(),
				command.ErrOrStderr(),
				append([]string{"volume", "rm"}, volumeNames...)...,
			)
		},
	}
	command.Flags().BoolVar(
		&confirmDestroyData,
		"confirm-destroy-data",
		false,
		"confirm deletion of Local named volumes",
	)
	return command
}

func composeArgs(options Options, profiles []string, action ...string) []string {
	args := []string{
		"compose",
		"-p", options.projectName(),
		"--env-file", options.envFile(),
		"--file", "compose.local.yaml",
	}
	for _, profile := range profiles {
		args = append(args, "--profile", profile)
	}
	return append(args, action...)
}

func runCompose(options Options, command *cobra.Command, profiles []string, action ...string) error {
	return runComposeWithOutput(
		options,
		command,
		profiles,
		command.OutOrStdout(),
		command.ErrOrStderr(),
		action...,
	)
}

func runComposeWithOutput(
	options Options,
	command *cobra.Command,
	profiles []string,
	stdout io.Writer,
	stderr io.Writer,
	action ...string,
) error {
	if err := validateProjectName(options.projectName()); err != nil {
		return err
	}
	if err := ensureLocalEnv(options); err != nil {
		return err
	}
	return options.Run(command.Context(), Command{
		Name:   "docker",
		Args:   composeArgs(options, profiles, action...),
		Dir:    options.RepoRoot,
		Stdin:  command.InOrStdin(),
		Stdout: stdout,
		Stderr: stderr,
	})
}

func runDockerWithOutput(
	options Options,
	command *cobra.Command,
	stdout io.Writer,
	stderr io.Writer,
	args ...string,
) error {
	return options.Run(command.Context(), Command{
		Name:   "docker",
		Args:   args,
		Dir:    options.RepoRoot,
		Stdin:  command.InOrStdin(),
		Stdout: stdout,
		Stderr: stderr,
	})
}

func resolveNamedVolumes(options Options, command *cobra.Command) ([]string, error) {
	var volumeNames []string
	for _, volumeKey := range []string{"postgres_data", "seaweedfs_data", "dbgate_data"} {
		output := new(bytes.Buffer)
		if err := runDockerWithOutput(
			options,
			command,
			output,
			command.ErrOrStderr(),
			"volume", "ls",
			"--filter", "label=com.docker.compose.project="+options.projectName(),
			"--filter", "label=com.docker.compose.volume="+volumeKey,
			"--format", "{{.Name}}",
		); err != nil {
			return nil, fmt.Errorf("resolve Local named volume %s: %w", volumeKey, err)
		}
		matches := strings.Fields(output.String())
		if len(matches) > 1 {
			return nil, fmt.Errorf(
				"more than one Local volume is labeled for %s; refusing reset",
				volumeKey,
			)
		}
		if len(matches) == 1 {
			volumeNames = append(volumeNames, matches[0])
		}
	}
	return volumeNames, nil
}

func ensureLocalEnv(options Options) error {
	envFile := localEnvPath(options)
	info, err := os.Stat(envFile)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Local environment file does not exist: %s", envFile)
	}
	if err != nil {
		return fmt.Errorf("inspect Local environment file %s: %w", envFile, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Local environment path is not a regular file: %s", envFile)
	}
	return nil
}

func localEnvPath(options Options) string {
	envFile := options.envFile()
	if filepath.IsAbs(envFile) {
		return envFile
	}
	return filepath.Join(options.RepoRoot, envFile)
}

func createLocalEnv(envFile string) error {
	secrets := make([]string, 4)
	for index := range secrets {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return fmt.Errorf("generate Local secret: %w", err)
		}
		secrets[index] = hex.EncodeToString(bytes)
	}
	content := fmt.Sprintf(
		"XDN_LOCAL_POSTGRES_PORT=5432\n"+
			"XDN_LOCAL_POSTGRES_ADMIN_PASSWORD=%s\n"+
			"XDN_GATEWAY_MIGRATOR_PASSWORD=%s\n"+
			"XDN_GATEWAY_RUNTIME_PASSWORD=%s\n"+
			"XDN_LOCAL_SEAWEEDFS_PORT=8333\n"+
			"XDN_LOCAL_TEMPORAL_PORT=7233\n"+
			"XDN_LOCAL_DBGATE_PORT=3000\n"+
			"XDN_LOCAL_GATEWAY_PORT=3001\n"+
			"XDN_GATEWAY_AUTH_SECRET=%s\n",
		secrets[0],
		secrets[1],
		secrets[2],
		secrets[3],
	)

	file, err := os.OpenFile(envFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite existing Local environment file: %s", envFile)
	}
	if err != nil {
		return fmt.Errorf("create Local environment file %s: %w", envFile, err)
	}
	removeIncomplete := true
	defer func() {
		_ = file.Close()
		if removeIncomplete {
			_ = os.Remove(envFile)
		}
	}()
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("write Local environment file %s: %w", envFile, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Local environment file %s: %w", envFile, err)
	}
	removeIncomplete = false
	return nil
}

func (options Options) projectName() string {
	if options.ProjectName != "" {
		return options.ProjectName
	}
	return localProjectName
}

func (options Options) envFile() string {
	if options.EnvFile != "" {
		return options.EnvFile
	}
	return "config/local.env"
}

func validateProjectName(projectName string) error {
	if projectName != localProjectName && projectName != smokeProjectName {
		return fmt.Errorf(
			"Local Compose project must be %s or %s",
			localProjectName,
			smokeProjectName,
		)
	}
	return nil
}

func validatePassThrough(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf(
			"pass-through global options are disabled; xdd manages the Compose project, file, env file, and profiles",
		)
	}
	if args[0] != "down" && args[0] != "rm" {
		return nil
	}
	for _, arg := range args[1:] {
		if isVolumeDeletionFlag(arg) {
			return fmt.Errorf(
				"pass-through volume deletion is disabled; use reset --confirm-destroy-data",
			)
		}
	}
	return nil
}

func isVolumeDeletionFlag(arg string) bool {
	if arg == "--volumes" || strings.HasPrefix(arg, "--volumes=") {
		return true
	}
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return false
	}
	shortFlags, _, _ := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
	return strings.Contains(shortFlags, "v")
}

func preflightFullSources(repoRoot string) error {
	for _, source := range []string{"gateway", "pepmimic-mcp", "graphpep-mcp", "bindcraft-mcp"} {
		sourceDir := filepath.Clean(filepath.Join(repoRoot, "..", source))
		info, err := os.Stat(sourceDir)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("full profile source directory does not exist: %s", sourceDir)
		}
		if err != nil {
			return fmt.Errorf("inspect full profile source directory %s: %w", sourceDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("full profile source path is not a directory: %s", sourceDir)
		}

		dockerfile := filepath.Join(sourceDir, "Dockerfile")
		info, err = os.Stat(dockerfile)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("full profile Dockerfile does not exist: %s", dockerfile)
		}
		if err != nil {
			return fmt.Errorf("inspect full profile Dockerfile %s: %w", dockerfile, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("full profile Dockerfile is not a regular file: %s", dockerfile)
		}
	}
	return nil
}
