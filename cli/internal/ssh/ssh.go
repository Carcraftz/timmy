package ssh

import (
	"context"
	"os"
	"os/exec"
)

type Runner interface {
	Run(ctx context.Context, destination string) error
}

type OpenSSHRunner struct {
	Binary string
}

func NewRunner() OpenSSHRunner {
	return OpenSSHRunner{Binary: "ssh"}
}

func (r OpenSSHRunner) Run(ctx context.Context, destination string) error {
	binary := r.Binary
	if binary == "" {
		binary = "ssh"
	}

	cmd := exec.CommandContext(ctx, binary, destination)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
