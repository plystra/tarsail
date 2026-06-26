package localcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/plystra/tarsail/internal/ui"
)

type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
	Env    []string
}

type CommandError struct {
	Area     string
	Name     string
	Run      string
	Dir      string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	label := strings.TrimSpace(e.Name)
	if label == "" {
		label = e.Run
	}
	detail := strings.TrimSpace(ui.Redact(e.Stderr))
	if detail == "" && e.Err != nil {
		detail = e.Err.Error()
	}
	if len(detail) > 1200 {
		detail = detail[:1200] + "\n[output truncated]"
	}
	return fmt.Sprintf("[%s] Build step failed: %s\n\nCommand:\n  %s\n\nWorking directory:\n  %s\n\nDetails:\n  %s", e.Area, label, e.Run, e.Dir, indent(detail))
}

func (r Runner) Run(ctx context.Context, area, name, run, dir string) error {
	shell, args := shellCommand(run)
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), r.Env...)
	var stderr bytes.Buffer
	if r.Stdout != nil {
		cmd.Stdout = ui.RedactingWriter{Writer: r.Stdout}
	}
	cmd.Stderr = io.MultiWriter(&stderr)
	if r.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, ui.RedactingWriter{Writer: r.Stderr})
	}
	if err := cmd.Run(); err != nil {
		return commandError(area, name, run, dir, stderr.String(), err)
	}
	return nil
}

func shellCommand(run string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", run}
	}
	return "sh", []string{"-c", run}
}

func commandError(area, name, run, dir, stderr string, err error) error {
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
		Name:     name,
		Run:      run,
		Dir:      dir,
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

func indent(value string) string {
	return strings.ReplaceAll(value, "\n", "\n  ")
}
