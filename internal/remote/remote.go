package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/plystra/tarsail/internal/config"
	"github.com/plystra/tarsail/internal/release"
	"github.com/plystra/tarsail/internal/ui"
)

type Client struct {
	Target         config.Target
	Project        string
	ComposeEnvFile string
	Secrets        []config.SecretFile
	Stdout         io.Writer
	Stderr         io.Writer
	Verbose        bool
}

func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (c Client) address() string {
	return c.Target.User + "@" + c.Target.Host
}

func (c Client) sshArgs(remoteCommand string) []string {
	return []string{
		"-p", strconv.Itoa(c.Target.Port),
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		c.address(),
		remoteCommand,
	}
}

func (c Client) Capture(ctx context.Context, area, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", c.sshArgs("sh -c "+ShellQuote(script))...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), c.commandError(area, script, stdout.String(), stderr.String(), err)
	}
	return stdout.String(), nil
}

func (c Client) Stream(ctx context.Context, area, script string) error {
	cmd := exec.CommandContext(ctx, "ssh", c.sshArgs("sh -c "+ShellQuote(script))...)
	if c.Stdout != nil {
		cmd.Stdout = ui.RedactingWriter{Writer: c.Stdout}
	}
	if c.Stderr != nil {
		cmd.Stderr = ui.RedactingWriter{Writer: c.Stderr}
	}
	if err := cmd.Run(); err != nil {
		return c.commandError(area, script, "", "", err)
	}
	return nil
}

func (c Client) commandError(area, script, stdout, stderr string, err error) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if detail == "" {
		detail = err.Error()
	}
	detail = ui.Redact(detail)
	if len(detail) > 1200 {
		detail = detail[:1200] + "\n[output truncated]"
	}
	return fmt.Errorf("[%s] Remote command failed on %s.\n\nDetails:\n  %s", area, c.address(), strings.ReplaceAll(detail, "\n", "\n  "))
}

func (c Client) CheckSSH(ctx context.Context) error {
	_, err := c.Capture(ctx, "remote:ssh", "true")
	return err
}

func (c Client) EnsureLayout(ctx context.Context) error {
	target := ShellQuote(c.Target.Path)
	script := fmt.Sprintf(`set -eu
mkdir -p %s
cd %s
mkdir -p releases incoming shared
test -w .
test -w releases
test -w incoming`, target, target)
	_, err := c.Capture(ctx, "remote:path", script)
	return err
}

func (c Client) CheckSharedFiles(ctx context.Context, targets []string) error {
	if len(targets) == 0 {
		return nil
	}
	target := ShellQuote(c.Target.Path)
	var tests []string
	for _, item := range uniqueStrings(targets) {
		tests = append(tests, fmt.Sprintf("test -f %s", ShellQuote(item)))
	}
	script := fmt.Sprintf(`set -eu
cd %s
%s`, target, strings.Join(tests, "\n"))
	_, err := c.Capture(ctx, "remote:shared", script)
	return err
}

func (c Client) CheckDocker(ctx context.Context) error {
	_, err := c.Capture(ctx, "remote:docker", "command -v docker >/dev/null 2>&1 && docker ps >/dev/null")
	return err
}

func (c Client) CheckCompose(ctx context.Context) error {
	_, err := c.Capture(ctx, "remote:compose", "docker compose version >/dev/null")
	return err
}

func (c Client) Upload(ctx context.Context, localPath, bundleName string) error {
	remotePath := path.Join(c.Target.Path, "incoming", bundleName)
	cmd := exec.CommandContext(ctx, "scp",
		"-P", strconv.Itoa(c.Target.Port),
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		localPath,
		c.scpDestination(remotePath),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(ui.Redact(stderr.String()))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("[remote:upload] Could not upload bundle to %s:%s.\n\nDetails:\n  %s", c.address(), remotePath, strings.ReplaceAll(detail, "\n", "\n  "))
	}
	return nil
}

func (c Client) UploadSecret(ctx context.Context, localPath string, secret config.SecretFile) error {
	remotePath := path.Join(c.Target.Path, secret.Target)
	remoteDir := path.Dir(remotePath)
	if _, err := c.Capture(ctx, "remote:secret-dir", "mkdir -p "+ShellQuote(remoteDir)); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "scp",
		"-P", strconv.Itoa(c.Target.Port),
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		localPath,
		c.scpDestination(remotePath),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(ui.Redact(stderr.String()))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("[remote:secret] Could not upload configured secret file to %s:%s.\n\nDetails:\n  %s", c.address(), remotePath, strings.ReplaceAll(detail, "\n", "\n  "))
	}
	if _, err := c.Capture(ctx, "remote:secret-mode", "chmod "+secret.Mode+" "+ShellQuote(remotePath)); err != nil {
		return err
	}
	return nil
}

func (c Client) scpDestination(remotePath string) string {
	return c.address() + ":" + remotePath
}

func (c Client) PrepareRelease(ctx context.Context, releaseID, bundleName string) error {
	if err := release.ValidateID(releaseID); err != nil {
		return err
	}
	target := ShellQuote(c.Target.Path)
	releaseIDQ := ShellQuote(releaseID)
	bundleNameQ := ShellQuote(bundleName)
	script := fmt.Sprintf(`set -eu
cd %s
mkdir -p releases incoming shared
bundle=%s
release_id=%s
bundle_path="incoming/$bundle"
release_dir="releases/$release_id"
test -f "$bundle_path"
if [ -e "$release_dir" ]; then
  echo "release directory already exists: $release_dir" >&2
  exit 1
fi
entries=$(tar -tzf "$bundle_path")
for entry in $entries; do
  case "$entry" in
    ""|/*|..|../*|*/../*|*/..)
      echo "unsafe archive entry: $entry" >&2
      exit 1
      ;;
  esac
done
mkdir -p "$release_dir"
tar -xzf "$bundle_path" -C "$release_dir"
test -f "$release_dir/manifest.json"
test -f "$release_dir/compose.yaml"
printf 'TARSAIL_RELEASE_ID=%%s\n' "$release_id" > "$release_dir/.tarsail.env"
ln -sfn ../../shared "$release_dir/shared"`, target, bundleNameQ, releaseIDQ)
	_, err := c.Capture(ctx, "remote:extract", script)
	return err
}

func (c Client) LoadImages(ctx context.Context, releaseID string) error {
	if err := release.ValidateID(releaseID); err != nil {
		return err
	}
	target := ShellQuote(c.Target.Path)
	releaseIDQ := ShellQuote(releaseID)
	script := fmt.Sprintf(`set -eu
cd %s
release_id=%s
cd "releases/$release_id"
found=0
for image in images/*.tar; do
  [ -f "$image" ] || continue
  found=1
  docker load -i "$image"
done
if [ "$found" -ne 1 ]; then
  echo "no image tar files found in release" >&2
  exit 1
fi`, target, releaseIDQ)
	return c.Stream(ctx, "remote:docker-load", script)
}

func (c Client) Activate(ctx context.Context, releaseID string) error {
	if err := release.ValidateID(releaseID); err != nil {
		return err
	}
	target := ShellQuote(c.Target.Path)
	releaseIDQ := ShellQuote(releaseID)
	script := fmt.Sprintf(`set -eu
cd %s
release_id=%s
test -d "releases/$release_id"
if [ -e current ] && [ ! -L current ]; then
  echo "current exists and is not a symlink" >&2
  exit 1
fi
ln -sfn "releases/$release_id" current`, target, releaseIDQ)
	_, err := c.Capture(ctx, "remote:activate", script)
	return err
}

func (c Client) ComposeUp(ctx context.Context) error {
	target := ShellQuote(c.Target.Path)
	project := ShellQuote(c.Project)
	args := c.composeCommandArgs("up -d")
	script := fmt.Sprintf(`set -eu
cd %s
test -L current
docker compose -p %s %s`, target, project, args)
	return c.Stream(ctx, "remote:compose-up", script)
}

func (c Client) ComposeUpAndStatus(ctx context.Context) error {
	if err := c.ComposeUp(ctx); err != nil {
		return err
	}
	return c.Status(ctx)
}

func (c Client) Status(ctx context.Context) error {
	target := ShellQuote(c.Target.Path)
	project := ShellQuote(c.Project)
	args := c.composeCommandArgs("ps")
	script := fmt.Sprintf(`set -eu
cd %s
test -L current
docker compose -p %s %s`, target, project, args)
	return c.Stream(ctx, "remote:status", script)
}

func (c Client) Logs(ctx context.Context, service string, tail int, follow bool) error {
	target := ShellQuote(c.Target.Path)
	project := ShellQuote(c.Project)
	tailArg := ShellQuote("--tail=" + strconv.Itoa(tail))
	followArg := ""
	if follow {
		followArg = " -f"
	}
	serviceArg := ""
	if service != "" {
		serviceArg = " " + ShellQuote(service)
	}
	args := c.composeCommandArgs("logs " + tailArg + followArg + serviceArg)
	script := fmt.Sprintf(`set -eu
cd %s
test -L current
docker compose -p %s %s`, target, project, args)
	return c.Stream(ctx, "remote:logs", script)
}

func (c Client) CurrentReleaseID(ctx context.Context) (string, error) {
	target := ShellQuote(c.Target.Path)
	script := fmt.Sprintf(`set -eu
cd %s
test -L current
target=$(readlink current)
case "$target" in
  releases/*)
    basename "$target"
    ;;
  *)
    echo "current does not point inside releases" >&2
    exit 1
    ;;
esac`, target)
	output, err := c.Capture(ctx, "remote:current", script)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(output)
	if err := release.ValidateID(id); err != nil {
		return "", err
	}
	return id, nil
}

func (c Client) ListReleases(ctx context.Context) ([]release.Release, error) {
	target := ShellQuote(c.Target.Path)
	script := fmt.Sprintf(`set -eu
cd %s
[ -d releases ] || exit 0
for d in releases/*; do
  [ -d "$d" ] || continue
  [ -f "$d/manifest.json" ] || continue
  id=${d#releases/}
  created=$(sed -n 's/.*"created_at"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$d/manifest.json" | head -n 1)
  [ -n "$created" ] || continue
  printf '%%s\t%%s\n' "$id" "$created"
done`, target)
	output, err := c.Capture(ctx, "remote:releases", script)
	if err != nil {
		return nil, err
	}
	return release.ParseList(output)
}

func (c Client) ActivateExistingRelease(ctx context.Context, releaseID string) error {
	if err := release.ValidateID(releaseID); err != nil {
		return err
	}
	target := ShellQuote(c.Target.Path)
	releaseIDQ := ShellQuote(releaseID)
	script := fmt.Sprintf(`set -eu
cd %s
release_id=%s
test -f "releases/$release_id/manifest.json"
test -f "releases/$release_id/compose.yaml"
ln -sfn ../../shared "releases/$release_id/shared"
cd "releases/$release_id"
found=0
for image in images/*.tar; do
  [ -f "$image" ] || continue
  found=1
  docker load -i "$image"
done
if [ "$found" -ne 1 ]; then
  echo "no image tar files found in release" >&2
  exit 1
fi
cd ../..
if [ -e current ] && [ ! -L current ]; then
  echo "current exists and is not a symlink" >&2
  exit 1
fi
ln -sfn "releases/$release_id" current`, target, releaseIDQ)
	return c.Stream(ctx, "remote:rollback", script)
}

func (c Client) RemoveReleases(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := release.ValidateID(id); err != nil {
			return err
		}
	}
	if len(ids) == 0 {
		return nil
	}

	target := ShellQuote(c.Target.Path)
	var quotedIDs []string
	for _, id := range ids {
		quotedIDs = append(quotedIDs, ShellQuote(id))
	}
	script := fmt.Sprintf(`set -eu
cd %s
for release_id in %s; do
  case "$release_id" in
    [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]-[a-f0-9][a-f0-9][a-f0-9][a-f0-9])
      ;;
    *)
      echo "unsafe release ID: $release_id" >&2
      exit 1
      ;;
  esac
  test -f "releases/$release_id/manifest.json"
  rm -rf -- "releases/$release_id"
done`, target, strings.Join(quotedIDs, " "))
	_, err := c.Capture(ctx, "remote:prune", script)
	return err
}

func (c Client) composeCommandArgs(command string) string {
	args := "--env-file current/.tarsail.env"
	if c.ComposeEnvFile != "" {
		args += " --env-file " + ShellQuote(c.ComposeEnvFile)
	}
	args += " -f current/compose.yaml"
	return args + " " + command
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var result []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
