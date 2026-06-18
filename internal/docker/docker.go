package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/plystra/tarsail/internal/ui"
)

type Runner struct {
	Dir     string
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
}

type CommandError struct {
	Area     string
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	detail := strings.TrimSpace(ui.Redact(e.Stderr))
	if detail == "" && e.Err != nil {
		detail = e.Err.Error()
	}
	if len(detail) > 1200 {
		detail = detail[:1200] + "\n[output truncated]"
	}
	return fmt.Sprintf("[%s] Command failed: docker %s\n\nDetails:\n  %s", e.Area, strings.Join(e.Args, " "), indent(detail))
}

func indent(value string) string {
	return strings.ReplaceAll(value, "\n", "\n  ")
}

func CheckDockerCommand() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("[local:docker] Docker command not found.\n\nHow to fix:\n  Install Docker Engine or Docker Desktop and ensure \"docker\" is on PATH.")
	}
	return nil
}

func (r Runner) Capture(ctx context.Context, area string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.Dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), commandError(area, args, stderr.String(), err)
	}
	return stdout.String(), nil
}

func (r Runner) Stream(ctx context.Context, area string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.Dir
	if r.Stdout != nil {
		cmd.Stdout = r.Stdout
	}
	if r.Stderr != nil {
		cmd.Stderr = r.Stderr
	}
	if err := cmd.Run(); err != nil {
		return commandError(area, args, "", err)
	}
	return nil
}

func commandError(area string, args []string, stderr string, err error) error {
	exitCode := -1
	var exitErr *exec.ExitError
	if ok := errorAs(err, &exitErr); ok {
		exitCode = exitErr.ExitCode()
		if stderr == "" {
			stderr = string(exitErr.Stderr)
		}
	}
	return &CommandError{
		Area:     area,
		Args:     args,
		ExitCode: exitCode,
		Stderr:   stderr,
		Err:      err,
	}
}

func errorAs(err error, target any) bool {
	switch t := target.(type) {
	case **exec.ExitError:
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			*t = exitErr
		}
		return ok
	default:
		return false
	}
}

func (r Runner) ComposeVersion(ctx context.Context) error {
	_, err := r.Capture(ctx, "local:compose", "compose", "version")
	return err
}

func (r Runner) DockerPS(ctx context.Context) error {
	_, err := r.Capture(ctx, "local:docker", "ps")
	return err
}

func (r Runner) ComposeConfig(ctx context.Context, composeFile, project string) (string, error) {
	return r.Capture(ctx, "compose:config", "compose", "-p", project, "-f", composeFile, "config")
}

func (r Runner) ComposeBuild(ctx context.Context, composeFile, project string) error {
	return r.Stream(ctx, "compose:build", "compose", "-p", project, "-f", composeFile, "build")
}

func (r Runner) ImageExists(ctx context.Context, image string) error {
	_, err := r.Capture(ctx, "docker:image", "image", "inspect", image)
	if err != nil {
		return fmt.Errorf("[docker:image] Image is not available locally: %s\n\nWhy it matters:\n  Tarsail bundles images from the local Docker daemon so the server does not need to pull them.\n\nHow to fix:\n  Build or pull the image locally, then run the command again.\n\nDetails:\n  %w", image, err)
	}
	return nil
}

func (r Runner) ImageSave(ctx context.Context, image, outputPath string) error {
	if err := r.Stream(ctx, "docker:save", "image", "save", "-o", outputPath, image); err != nil {
		return fmt.Errorf("[docker:save] Could not save image %s.\n\nDetails:\n  %w", image, err)
	}
	return nil
}
