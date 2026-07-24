package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestLocalDevCheckValidatesTheRenderedComposeConfiguration(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	var commands []Command
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, command Command) error {
			commands = append(commands, command)
			return nil
		},
	})
	stdout := new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "check"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute xdd local dev check: %v", err)
	}

	if got, want := len(commands), 1; got != want {
		t.Fatalf("command count: want %d, got %d", want, got)
	}
	wantArgs := []string{
		"compose",
		"-p", "xdenovo-platform-local",
		"--env-file", "config/local.env",
		"--file", "compose.local.yaml",
		"--profile", "dev",
		"config", "--quiet",
	}
	if commands[0].Name != "docker" || commands[0].Dir != repoRoot || !reflect.DeepEqual(commands[0].Args, wantArgs) {
		t.Fatalf("command mismatch\nwant: docker %#v in %q\ngot:  %s %#v in %q", wantArgs, repoRoot, commands[0].Name, commands[0].Args, commands[0].Dir)
	}
	if got, want := stdout.String(), "Local dev Compose configuration is valid.\n"; got != want {
		t.Fatalf("stdout mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestLocalFullCheckEnablesDevAndFullComposeProfiles(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	var commands []Command
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, command Command) error {
			commands = append(commands, command)
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "full", "check"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute xdd local full check: %v", err)
	}

	if got, want := len(commands), 1; got != want {
		t.Fatalf("command count: want %d, got %d", want, got)
	}
	wantArgs := []string{
		"compose",
		"-p", "xdenovo-platform-local",
		"--env-file", "config/local.env",
		"--file", "compose.local.yaml",
		"--profile", "dev",
		"--profile", "full",
		"config", "--quiet",
	}
	if commands[0].Name != "docker" || !reflect.DeepEqual(commands[0].Args, wantArgs) {
		t.Fatalf("command mismatch\nwant: docker %#v\ngot:  %s %#v", wantArgs, commands[0].Name, commands[0].Args)
	}
}

func TestLocalActionFailsClosedWhenTheEnvironmentFileIsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	runCalled := false
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, _ Command) error {
			runCalled = true
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "check"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing Local environment file to fail")
	}
	if want := filepath.Join(repoRoot, "config", "local.env"); !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not name missing file %q", err, want)
	}
	if runCalled {
		t.Fatal("docker must not run when config/local.env is missing")
	}
}

func TestLocalDevLifecycleActionsPreserveDataOnDown(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		actionArgs []string
	}{
		{name: "up waits for health", action: "up", actionArgs: []string{"up", "-d", "--wait"}},
		{name: "down preserves volumes", action: "down", actionArgs: []string{"down", "--remove-orphans"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeLocalEnv(t, repoRoot)

			var commands []Command
			root := New(Options{
				RepoRoot: repoRoot,
				Run: func(_ context.Context, command Command) error {
					commands = append(commands, command)
					return nil
				},
			})
			root.SetOut(new(bytes.Buffer))
			root.SetErr(new(bytes.Buffer))
			root.SetArgs([]string{"local", "dev", test.action})

			if err := root.Execute(); err != nil {
				t.Fatalf("execute xdd local dev %s: %v", test.action, err)
			}

			if got, want := len(commands), 1; got != want {
				t.Fatalf("command count: want %d, got %d", want, got)
			}
			wantArgs := append([]string{
				"compose",
				"-p", "xdenovo-platform-local",
				"--env-file", "config/local.env",
				"--file", "compose.local.yaml",
				"--profile", "dev",
			}, test.actionArgs...)
			if !reflect.DeepEqual(commands[0].Args, wantArgs) {
				t.Fatalf("args mismatch\nwant: %#v\ngot:  %#v", wantArgs, commands[0].Args)
			}
		})
	}
}

func TestLocalFullPassesArgumentsThroughAfterDoubleDash(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	var commands []Command
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, command Command) error {
			commands = append(commands, command)
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "full", "--", "logs", "--follow", "gateway"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute Compose pass-through: %v", err)
	}

	if got, want := len(commands), 1; got != want {
		t.Fatalf("command count: want %d, got %d", want, got)
	}
	wantArgs := []string{
		"compose",
		"-p", "xdenovo-platform-local",
		"--env-file", "config/local.env",
		"--file", "compose.local.yaml",
		"--profile", "dev",
		"--profile", "full",
		"logs", "--follow", "gateway",
	}
	if !reflect.DeepEqual(commands[0].Args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\ngot:  %#v", wantArgs, commands[0].Args)
	}
}

func TestLocalPassThroughCannotBypassConfirmedVolumeReset(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "long volume flag", args: []string{"down", "--volumes"}, wantErr: "reset --confirm-destroy-data"},
		{name: "short volume flag", args: []string{"down", "-v"}, wantErr: "reset --confirm-destroy-data"},
		{name: "assigned short volume flag", args: []string{"down", "-v=true"}, wantErr: "reset --confirm-destroy-data"},
		{name: "clustered short volume flag", args: []string{"rm", "-fsv"}, wantErr: "reset --confirm-destroy-data"},
		{name: "project override", args: []string{"--project-name", "production", "down"}, wantErr: "global options"},
		{name: "Compose file override", args: []string{"--file", "../production.yaml", "down"}, wantErr: "global options"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeLocalEnv(t, repoRoot)

			runCalled := false
			root := New(Options{
				RepoRoot: repoRoot,
				Run: func(_ context.Context, _ Command) error {
					runCalled = true
					return nil
				},
			})
			root.SetOut(new(bytes.Buffer))
			root.SetErr(new(bytes.Buffer))
			root.SetArgs(append([]string{"local", "dev", "--"}, test.args...))

			err := root.Execute()
			if err == nil {
				t.Fatal("expected unsafe pass-through to fail")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error %q does not contain %q", err, test.wantErr)
			}
			if runCalled {
				t.Fatal("docker must not run for unsafe pass-through")
			}
		})
	}
}

func TestLocalDevBootstrapWaitsForPostgresAndRunsTheOwnedBootstrap(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	var commands []Command
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, command Command) error {
			commands = append(commands, command)
			return nil
		},
	})
	stdout := new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "bootstrap"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute bootstrap: %v", err)
	}

	wantActions := [][]string{
		{"config", "--quiet"},
		{"exec", "--no-TTY", "postgres", "pg_isready", "--username", "xdenovo_bootstrap", "--dbname", "postgres"},
		{"exec", "--no-TTY", "postgres", "psql", "--username", "xdenovo_bootstrap", "--dbname", "postgres", "--set", "ON_ERROR_STOP=1", "--file", "/opt/xdenovo/bootstrap-platform.sql"},
	}
	if got, want := len(commands), len(wantActions); got != want {
		t.Fatalf("command count: want %d, got %d", want, got)
	}
	wantBase := []string{
		"compose",
		"-p", "xdenovo-platform-local",
		"--env-file", "config/local.env",
		"--file", "compose.local.yaml",
		"--profile", "dev",
	}
	for index, wantAction := range wantActions {
		wantArgs := append(append([]string{}, wantBase...), wantAction...)
		if got := commands[index].Args; !reflect.DeepEqual(got, wantArgs) {
			t.Fatalf("command %d args mismatch\nwant: %#v\ngot:  %#v", index, wantArgs, got)
		}
	}
	if got, want := stdout.String(), "Local PostgreSQL bootstrap is complete.\n"; got != want {
		t.Fatalf("stdout mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestLocalResetRefusesWithoutExplicitConfirmation(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	runCalled := false
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, _ Command) error {
			runCalled = true
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "reset"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected reset without confirmation to fail")
	}
	if !strings.Contains(err.Error(), "--confirm-destroy-data") {
		t.Fatalf("error %q does not explain the required confirmation flag", err)
	}
	if runCalled {
		t.Fatal("docker must not run before destructive reset is confirmed")
	}
}

func TestLocalResetDisplaysAndDeletesOnlyLabelResolvedNamedVolumes(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	var commands []Command
	stdout := new(bytes.Buffer)
	volumeNames := map[string]string{
		"postgres_data":  "xdenovo-platform-local_postgres_data",
		"seaweedfs_data": "xdenovo-platform-local_seaweedfs_data",
		"dbgate_data":    "xdenovo-platform-local_dbgate_data",
	}
	root := New(Options{
		RepoRoot: repoRoot,
		Run: func(_ context.Context, command Command) error {
			if len(command.Args) >= 2 && command.Args[0] == "volume" && command.Args[1] == "ls" {
				for _, arg := range command.Args {
					const prefix = "label=com.docker.compose.volume="
					if strings.HasPrefix(arg, prefix) {
						_, _ = fmt.Fprintln(command.Stdout, volumeNames[strings.TrimPrefix(arg, prefix)])
					}
				}
			}
			if slicesContainSequence(command.Args, []string{"down", "--remove-orphans"}) {
				if !strings.Contains(stdout.String(), "xdenovo-platform-local_postgres_data") {
					t.Fatal("reset targets must be displayed before docker compose down runs")
				}
			}
			commands = append(commands, command)
			return nil
		},
	})
	root.SetOut(stdout)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "reset", "--confirm-destroy-data"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute confirmed reset: %v", err)
	}

	if got, want := len(commands), 6; got != want {
		t.Fatalf("command count: want %d, got %d", want, got)
	}
	if !slicesContainSequence(commands[0].Args, []string{"config", "--quiet"}) {
		t.Fatalf("first command must validate Compose, got %#v", commands[0].Args)
	}
	for index, volumeKey := range []string{"postgres_data", "seaweedfs_data", "dbgate_data"} {
		command := commands[index+1]
		if command.Name != "docker" ||
			!slicesContainSequence(command.Args, []string{
				"volume", "ls",
				"--filter", "label=com.docker.compose.project=xdenovo-platform-local",
				"--filter", "label=com.docker.compose.volume=" + volumeKey,
				"--format", "{{.Name}}",
			}) {
			t.Fatalf("volume lookup %d mismatch: %#v", index, command)
		}
	}
	if !slicesContainSequence(commands[4].Args, []string{"down", "--remove-orphans"}) ||
		slicesContainSequence(commands[4].Args, []string{"--volumes"}) {
		t.Fatalf("reset down command may not delete volumes implicitly: %#v", commands[4].Args)
	}
	wantRemove := []string{
		"volume", "rm",
		"xdenovo-platform-local_postgres_data",
		"xdenovo-platform-local_seaweedfs_data",
		"xdenovo-platform-local_dbgate_data",
	}
	if commands[5].Name != "docker" || !reflect.DeepEqual(commands[5].Args, wantRemove) {
		t.Fatalf("exact volume removal mismatch\nwant: %#v\ngot:  %#v", wantRemove, commands[5].Args)
	}
	for _, want := range []string{
		"Compose project: xdenovo-platform-local",
		"xdenovo-platform-local_postgres_data",
		"xdenovo-platform-local_seaweedfs_data",
		"xdenovo-platform-local_dbgate_data",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("reset output %q does not contain %q", stdout.String(), want)
		}
	}
}

func slicesContainSequence(values, sequence []string) bool {
	if len(sequence) > len(values) {
		return false
	}
	for index := 0; index <= len(values)-len(sequence); index++ {
		if reflect.DeepEqual(values[index:index+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func TestLocalFullUpNamesMissingSiblingSourcesBeforeDockerRuns(t *testing.T) {
	tests := []struct {
		name        string
		prepare     func(*testing.T, string)
		missingPath func(string) string
	}{
		{
			name:    "missing repository",
			prepare: func(*testing.T, string) {},
			missingPath: func(workspace string) string {
				return filepath.Join(workspace, "gateway")
			},
		},
		{
			name: "missing Dockerfile",
			prepare: func(t *testing.T, workspace string) {
				for _, source := range []string{"gateway", "pepmimic-mcp", "graphpep-mcp", "bindcraft-mcp"} {
					sourceDir := filepath.Join(workspace, source)
					if err := os.MkdirAll(sourceDir, 0o755); err != nil {
						t.Fatalf("create source directory: %v", err)
					}
					if source != "graphpep-mcp" {
						if err := os.WriteFile(filepath.Join(sourceDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
							t.Fatalf("write source Dockerfile: %v", err)
						}
					}
				}
			},
			missingPath: func(workspace string) string {
				return filepath.Join(workspace, "graphpep-mcp", "Dockerfile")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			repoRoot := filepath.Join(workspace, "platform-deploy")
			if err := os.MkdirAll(repoRoot, 0o755); err != nil {
				t.Fatalf("create repository root: %v", err)
			}
			writeLocalEnv(t, repoRoot)
			test.prepare(t, workspace)

			runCalled := false
			root := New(Options{
				RepoRoot: repoRoot,
				Run: func(_ context.Context, _ Command) error {
					runCalled = true
					return nil
				},
			})
			root.SetOut(new(bytes.Buffer))
			root.SetErr(new(bytes.Buffer))
			root.SetArgs([]string{"local", "full", "up"})

			err := root.Execute()
			if err == nil {
				t.Fatal("expected missing full-profile source to fail")
			}
			if want := test.missingPath(workspace); !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not name missing path %q", err, want)
			}
			if runCalled {
				t.Fatal("docker must not run before full-profile sources pass preflight")
			}
		})
	}
}

func TestLocalRejectsProfilesOutsideDevAndFull(t *testing.T) {
	runCalled := false
	root := New(Options{
		RepoRoot: t.TempDir(),
		Run: func(_ context.Context, _ Command) error {
			runCalled = true
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "production", "up"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported Local profile to fail")
	}
	if !strings.Contains(err.Error(), "production") {
		t.Fatalf("error %q does not name unsupported profile", err)
	}
	if runCalled {
		t.Fatal("docker must not run for an unsupported profile")
	}
}

func TestLocalCommandsUseExplicitSmokeIsolation(t *testing.T) {
	repoRoot := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "smoke.env")
	if err := os.WriteFile(envFile, []byte("test=true\n"), 0o600); err != nil {
		t.Fatalf("write smoke environment file: %v", err)
	}

	var commands []Command
	root := New(Options{
		RepoRoot:    repoRoot,
		ProjectName: "xdenovo-platform-local-smoke",
		EnvFile:     envFile,
		Run: func(_ context.Context, command Command) error {
			commands = append(commands, command)
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "check"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute isolated smoke check: %v", err)
	}

	if got, want := len(commands), 1; got != want {
		t.Fatalf("command count: want %d, got %d", want, got)
	}
	wantArgs := []string{
		"compose",
		"-p", "xdenovo-platform-local-smoke",
		"--env-file", envFile,
		"--file", "compose.local.yaml",
		"--profile", "dev",
		"config", "--quiet",
	}
	if !reflect.DeepEqual(commands[0].Args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\ngot:  %#v", wantArgs, commands[0].Args)
	}
}

func TestLocalRejectsProjectNamesOutsideStableLocalTargets(t *testing.T) {
	repoRoot := t.TempDir()
	writeLocalEnv(t, repoRoot)

	runCalled := false
	root := New(Options{
		RepoRoot:    repoRoot,
		ProjectName: "production",
		Run: func(_ context.Context, _ Command) error {
			runCalled = true
			return nil
		},
	})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"local", "dev", "check"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unrelated Compose project to fail")
	}
	if !strings.Contains(err.Error(), "xdenovo-platform-local-smoke") {
		t.Fatalf("error %q does not name the allowed smoke target", err)
	}
	if runCalled {
		t.Fatal("docker must not run for an unrelated Compose project")
	}
}

func TestLocalInitCreatesSecureRandomConfigurationAndRefusesOverwrite(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoRoot, "config"), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}

	runCalled := false
	newCommand := func() (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		root := New(Options{
			RepoRoot: repoRoot,
			Run: func(_ context.Context, _ Command) error {
				runCalled = true
				return nil
			},
		})
		root.SetOut(stdout)
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{"local", "dev", "init"})
		return stdout, root.Execute()
	}

	stdout, err := newCommand()
	if err != nil {
		t.Fatalf("execute Local init: %v", err)
	}
	envFile := filepath.Join(repoRoot, "config", "local.env")
	info, err := os.Stat(envFile)
	if err != nil {
		t.Fatalf("stat generated Local environment file: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("generated file mode: want %o, got %o", want, got)
	}

	content, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read generated Local environment file: %v", err)
	}
	values := parseEnvForTest(t, string(content))
	for key, want := range map[string]string{
		"XDN_LOCAL_POSTGRES_PORT":         "5432",
		"XDN_LOCAL_SEAWEEDFS_PORT":        "8333",
		"XDN_LOCAL_SEAWEEDFS_MASTER_PORT": "9333",
		"XDN_LOCAL_TEMPORAL_PORT":         "7233",
		"XDN_LOCAL_TEMPORAL_UI_PORT":      "8233",
		"XDN_LOCAL_DBGATE_PORT":           "3000",
		"XDN_LOCAL_GATEWAY_PORT":          "3001",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s: want %q, got %q", key, want, got)
		}
	}
	secretKeys := []string{
		"XDN_LOCAL_POSTGRES_ADMIN_PASSWORD",
		"XDN_GATEWAY_MIGRATOR_PASSWORD",
		"XDN_GATEWAY_RUNTIME_PASSWORD",
		"XDN_GATEWAY_AUTH_SECRET",
	}
	seenSecrets := make(map[string]bool)
	for _, key := range secretKeys {
		value := values[key]
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(value) {
			t.Fatalf("%s must be a 32-byte hexadecimal secret, got %q", key, value)
		}
		if seenSecrets[value] {
			t.Fatalf("%s duplicated another generated secret", key)
		}
		seenSecrets[value] = true
		if strings.Contains(stdout.String(), value) {
			t.Fatalf("stdout must not reveal %s", key)
		}
	}

	original := string(content)
	if _, err := newCommand(); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second init must refuse overwrite, got: %v", err)
	}
	content, err = os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read Local environment file after refused overwrite: %v", err)
	}
	if got := string(content); got != original {
		t.Fatal("refused overwrite changed the existing Local environment file")
	}
	if runCalled {
		t.Fatal("Local init must not invoke Docker")
	}
}

func parseEnvForTest(t *testing.T, content string) map[string]string {
	t.Helper()

	values := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("generated environment line has no equals sign: %q", line)
		}
		values[key] = value
	}
	return values
}

func writeLocalEnv(t *testing.T, repoRoot string) {
	t.Helper()

	configDir := filepath.Join(repoRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "local.env"), []byte("test=true\n"), 0o600); err != nil {
		t.Fatalf("write Local environment file: %v", err)
	}
}
