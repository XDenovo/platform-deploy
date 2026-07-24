package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Dir = command.Dir
	process.Stdin = command.Stdin
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	if err := process.Run(); err != nil {
		return fmt.Errorf("run %s: %w", strings.Join(append([]string{command.Name}, command.Args...), " "), err)
	}
	return nil
}
