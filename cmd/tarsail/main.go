package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/plystra/tarsail/internal/bundle"
	"github.com/plystra/tarsail/internal/compose"
	"github.com/plystra/tarsail/internal/config"
	localdocker "github.com/plystra/tarsail/internal/docker"
	"github.com/plystra/tarsail/internal/release"
	"github.com/plystra/tarsail/internal/remote"
	"github.com/plystra/tarsail/internal/ui"
)

type app struct {
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
	configPath   string
	identityFile string
	askPassword  bool
	sshPassword  string
	verbose      bool
	yes          bool
	showVersion  bool
}

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	application := app{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	if err := application.run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(application.stderr, err)
		os.Exit(1)
	}
}

func (a *app) run(ctx context.Context, args []string) error {
	global := flag.NewFlagSet("tarsail", flag.ContinueOnError)
	global.SetOutput(a.stderr)
	global.StringVar(&a.configPath, "config", config.DefaultFile, "path to tarsail.yml")
	global.StringVar(&a.identityFile, "identity-file", "", "SSH private key file for remote access")
	global.StringVar(&a.identityFile, "ssh-key", "", "alias for --identity-file")
	global.BoolVar(&a.askPassword, "ask-password", false, "prompt once for the remote user's SSH password")
	global.BoolVar(&a.verbose, "verbose", false, "show verbose command output where available")
	global.BoolVar(&a.yes, "yes", false, "answer yes to confirmation prompts")
	global.BoolVar(&a.showVersion, "version", false, "show Tarsail version")
	global.Usage = a.printUsage

	if err := global.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if a.showVersion {
		return a.cmdVersion()
	}
	rest := global.Args()
	if len(rest) == 0 {
		a.printUsage()
		return nil
	}

	command := rest[0]
	commandArgs := rest[1:]
	switch command {
	case "init":
		return a.cmdInit()
	case "doctor":
		return a.cmdDoctor(ctx)
	case "deploy":
		return a.cmdDeploy(ctx)
	case "status":
		return a.cmdStatus(ctx)
	case "logs":
		return a.cmdLogs(ctx, commandArgs)
	case "rollback":
		return a.cmdRollback(ctx)
	case "prune":
		return a.cmdPrune(ctx, commandArgs)
	case "version":
		return a.cmdVersion()
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	default:
		return fmt.Errorf("[cli:command] Unknown command: %s\n\nHow to fix:\n  Use one of: init, doctor, deploy, status, logs, rollback, prune, version.", command)
	}
}

func (a *app) printUsage() {
	fmt.Fprintln(a.stdout, `Tarsail

Usage:
  tarsail [--config tarsail.yml] [--identity-file ~/.ssh/id_ed25519] [--ask-password] [--verbose] [--yes] [--version] <command>

Commands:
  init       Create a minimal tarsail.yml
  doctor     Check local and remote readiness
  deploy     Build, bundle, upload, load, and start the app
  status     Show remote Compose status
  logs       Show remote Compose logs
  rollback   Roll back to the previous release
  prune      Delete old non-current releases
  version    Show Tarsail version`)
}

func (a *app) cmdVersion() error {
	fmt.Fprintln(a.stdout, "tarsail "+version)
	if a.verbose {
		fmt.Fprintln(a.stdout, "commit "+commit)
		fmt.Fprintln(a.stdout, "built "+date)
	}
	return nil
}

func (a *app) cmdInit() error {
	if err := config.Init(a.configPath); err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, "Created "+a.configPath)
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Next steps:")
	fmt.Fprintln(a.stdout, "  1. Edit target.host, target.user, and target.path")
	fmt.Fprintln(a.stdout, "  2. Ensure every built service has an explicit image tag")
	fmt.Fprintln(a.stdout, "  3. Run: tarsail doctor")
	return nil
}

func (a *app) loadProject() (*config.Project, error) {
	return config.Load(a.configPath)
}

func (a *app) dockerRunner(project *config.Project) localdocker.Runner {
	return a.dockerRunnerForRelease(project, "")
}

func (a *app) dockerRunnerForRelease(project *config.Project, releaseID string) localdocker.Runner {
	var env []string
	if releaseID != "" {
		env = append(env, "TARSAIL_RELEASE_ID="+releaseID)
	}
	return localdocker.Runner{
		Dir:     project.Root,
		Stdout:  a.stdout,
		Stderr:  a.stderr,
		Verbose: a.verbose,
		Env:     env,
	}
}

func (a *app) remoteClient(project *config.Project) (remote.Client, error) {
	if a.identityFile != "" && a.askPassword {
		return remote.Client{}, fmt.Errorf("[cli:auth] --identity-file and --ask-password cannot be used together.")
	}
	auth := remote.Auth{IdentityFile: a.identityFile}
	if auth.IdentityFile != "" {
		identityFile, err := resolveIdentityFile(auth.IdentityFile)
		if err != nil {
			return remote.Client{}, err
		}
		auth.IdentityFile = identityFile
	}
	if a.askPassword && a.sshPassword == "" {
		password, err := ui.PromptPassword(a.stdin, a.stdout, fmt.Sprintf("SSH password for %s@%s: ", project.Target.User, project.Target.Host))
		if err != nil {
			return remote.Client{}, err
		}
		if password == "" {
			return remote.Client{}, fmt.Errorf("[cli:auth] SSH password cannot be empty.")
		}
		a.sshPassword = password
	}
	auth.Password = a.sshPassword
	return remote.Client{
		Target:         project.Target,
		Project:        project.Project,
		ComposeEnvFile: project.ComposeEnvFileTarget(),
		Secrets:        project.Secrets,
		Auth:           auth,
		Stdout:         a.stdout,
		Stderr:         a.stderr,
		Verbose:        a.verbose,
	}, nil
}

func resolveIdentityFile(value string) (string, error) {
	if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("[cli:auth] Could not resolve home directory for --identity-file: %w", err)
		}
		value = filepath.Join(home, value[2:])
	}
	absPath, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("[cli:auth] Could not resolve --identity-file: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("[cli:auth] SSH identity file not found: %s", absPath)
		}
		return "", fmt.Errorf("[cli:auth] Could not inspect SSH identity file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("[cli:auth] SSH identity file points to a directory: %s", absPath)
	}
	return absPath, nil
}

func (a *app) cmdDoctor(ctx context.Context) error {
	project, err := a.loadProject()
	localErrs := 0
	remoteErrs := 0

	fmt.Fprintln(a.stdout, "Local:")
	if err != nil {
		ui.Fail(a.stdout, "config loaded")
		fmt.Fprintln(a.stderr, err)
		return err
	}
	ui.OK(a.stdout, "config loaded")
	ui.OK(a.stdout, "Compose file found: "+project.Compose.File)

	runner := a.dockerRunner(project)
	if err := localdocker.CheckDockerCommand(); err != nil {
		ui.Fail(a.stdout, "Docker command available")
		fmt.Fprintln(a.stderr, err)
		localErrs++
	} else {
		ui.OK(a.stdout, "Docker command available")
	}
	if err := runner.ComposeVersion(ctx); err != nil {
		ui.Fail(a.stdout, "Docker Compose available")
		fmt.Fprintln(a.stderr, err)
		localErrs++
	} else {
		ui.OK(a.stdout, "Docker Compose available")
	}
	if err := runner.DockerPS(ctx); err != nil {
		ui.Fail(a.stdout, "Docker daemon reachable")
		fmt.Fprintln(a.stderr, err)
		localErrs++
	} else {
		ui.OK(a.stdout, "Docker daemon reachable")
	}
	if _, err := discoverImages(ctx, project, runner, false); err != nil {
		ui.Fail(a.stdout, "Compose images discoverable")
		fmt.Fprintln(a.stderr, err)
		localErrs++
	} else {
		ui.OK(a.stdout, "Compose images discoverable")
	}

	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Remote:")
	client, err := a.remoteClient(project)
	if err != nil {
		return err
	}
	if err := client.CheckSSH(ctx); err != nil {
		ui.Fail(a.stdout, fmt.Sprintf("SSH connected: %s@%s:%d", project.Target.User, project.Target.Host, project.Target.Port))
		fmt.Fprintln(a.stderr, err)
		remoteErrs++
	} else {
		ui.OK(a.stdout, fmt.Sprintf("SSH connected: %s@%s:%d", project.Target.User, project.Target.Host, project.Target.Port))
	}
	if err := client.EnsureLayout(ctx); err != nil {
		ui.Fail(a.stdout, "target path writable: "+project.Target.Path)
		fmt.Fprintln(a.stderr, err)
		remoteErrs++
	} else {
		ui.OK(a.stdout, "target path writable: "+project.Target.Path)
	}
	if err := client.CheckSharedFiles(ctx, project.RequiredSharedTargets()); err != nil {
		ui.Fail(a.stdout, "shared files available")
		fmt.Fprintln(a.stderr, err)
		remoteErrs++
	} else {
		ui.OK(a.stdout, "shared files available")
	}
	if err := client.CheckDocker(ctx); err != nil {
		ui.Fail(a.stdout, "Docker available")
		fmt.Fprintln(a.stderr, err)
		remoteErrs++
	} else {
		ui.OK(a.stdout, "Docker available")
	}
	if err := client.CheckCompose(ctx); err != nil {
		ui.Fail(a.stdout, "Docker Compose available")
		fmt.Fprintln(a.stderr, err)
		remoteErrs++
	} else {
		ui.OK(a.stdout, "Docker Compose available")
	}

	if localErrs+remoteErrs > 0 {
		return fmt.Errorf("[doctor] %d check(s) failed.", localErrs+remoteErrs)
	}
	return nil
}

func (a *app) cmdDeploy(ctx context.Context) error {
	project, err := a.loadProject()
	if err != nil {
		return err
	}
	releaseID, err := release.NewID(time.Now())
	if err != nil {
		return err
	}
	createdAt := time.Now().UTC()

	fmt.Fprintln(a.stdout, "Tarsail deploy")
	fmt.Fprintln(a.stdout, "Project: "+project.Project)
	fmt.Fprintf(a.stdout, "Target: %s@%s:%s\n", project.Target.User, project.Target.Host, project.Target.Path)
	fmt.Fprintln(a.stdout, "Release: "+releaseID)
	fmt.Fprintln(a.stdout)

	runner := a.dockerRunnerForRelease(project, releaseID)
	client, err := a.remoteClient(project)
	if err != nil {
		return err
	}

	ui.Step(a.stdout, 1, 8, "Checking local environment")
	images, err := a.checkLocalForDeploy(ctx, project, runner)
	if err != nil {
		return err
	}

	ui.Step(a.stdout, 2, 8, "Checking remote server")
	if err := client.CheckSSH(ctx); err != nil {
		return err
	}
	if err := client.EnsureLayout(ctx); err != nil {
		return err
	}
	if err := client.CheckDocker(ctx); err != nil {
		return err
	}
	if err := client.CheckCompose(ctx); err != nil {
		return err
	}
	if err := uploadSecrets(ctx, project, client); err != nil {
		return err
	}
	if err := client.CheckSharedFiles(ctx, project.RuntimeSharedTargets()); err != nil {
		return err
	}

	ui.Step(a.stdout, 3, 8, "Building images")
	if err := runner.ComposeBuild(ctx, project.Compose.File, project.Project, project.ComposeEnvFileSourcePath()); err != nil {
		return err
	}
	images, err = discoverImages(ctx, project, runner, true)
	if err != nil {
		return err
	}

	ui.Step(a.stdout, 4, 8, "Creating bundle")
	bundleResult, err := bundle.Create(ctx, project, releaseID, createdAt, images, runner)
	if err != nil {
		return err
	}
	defer os.Remove(bundleResult.Path)
	bundleName := filepath.Base(bundleResult.Path)

	ui.Step(a.stdout, 5, 8, "Uploading bundle")
	if err := client.Upload(ctx, bundleResult.Path, bundleName); err != nil {
		return err
	}
	if err := client.PrepareRelease(ctx, releaseID, bundleName); err != nil {
		return err
	}

	ui.Step(a.stdout, 6, 8, "Loading images")
	if err := client.LoadImages(ctx, releaseID); err != nil {
		return err
	}

	ui.Step(a.stdout, 7, 8, "Starting Compose app")
	if err := client.Activate(ctx, releaseID); err != nil {
		return err
	}
	if err := client.ComposeUp(ctx); err != nil {
		return fmt.Errorf("%w\n\nDeployment failed after activation.\nRun \"tarsail rollback\" to restore the previous release.", err)
	}

	ui.Step(a.stdout, 8, 8, "Reading status")
	if err := client.Status(ctx); err != nil {
		return err
	}

	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Deployment complete.")
	return nil
}

func uploadSecrets(ctx context.Context, project *config.Project, client remote.Client) error {
	for _, secret := range project.SecretUploads() {
		if err := client.UploadSecret(ctx, project.LocalPath(secret.Source), secret); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) checkLocalForDeploy(ctx context.Context, project *config.Project, runner localdocker.Runner) ([]compose.ServiceImage, error) {
	if err := localdocker.CheckDockerCommand(); err != nil {
		return nil, err
	}
	if err := runner.ComposeVersion(ctx); err != nil {
		return nil, err
	}
	if err := runner.DockerPS(ctx); err != nil {
		return nil, err
	}
	return discoverImages(ctx, project, runner, false)
}

func discoverImages(ctx context.Context, project *config.Project, runner localdocker.Runner, requireLocal bool) ([]compose.ServiceImage, error) {
	output, err := runner.ComposeConfig(ctx, project.Compose.File, project.Project, project.ComposeEnvFileSourcePath())
	if err != nil {
		return nil, err
	}
	images, err := compose.DiscoverServiceImages(output)
	if err != nil {
		return nil, err
	}
	if requireLocal {
		for _, image := range images {
			if err := runner.ImageExists(ctx, image.Image); err != nil {
				return nil, err
			}
		}
	}
	return images, nil
}

func (a *app) cmdStatus(ctx context.Context) error {
	project, err := a.loadProject()
	if err != nil {
		return err
	}
	client, err := a.remoteClient(project)
	if err != nil {
		return err
	}
	return client.Status(ctx)
}

func (a *app) cmdLogs(ctx context.Context, args []string) error {
	logFlags := flag.NewFlagSet("logs", flag.ContinueOnError)
	logFlags.SetOutput(a.stderr)
	tail := logFlags.Int("tail", 200, "number of log lines to show")
	follow := logFlags.Bool("f", false, "follow logs")
	if err := logFlags.Parse(args); err != nil {
		return err
	}
	if *tail < 1 || *tail > 10000 {
		return fmt.Errorf("[logs:args] --tail must be between 1 and 10000.")
	}
	rest := logFlags.Args()
	if len(rest) > 1 {
		return fmt.Errorf("[logs:args] Usage: tarsail logs [--tail 200] [-f] [service]")
	}
	service := ""
	if len(rest) == 1 {
		service = rest[0]
		if err := validateServiceArg(service); err != nil {
			return err
		}
	}
	project, err := a.loadProject()
	if err != nil {
		return err
	}
	client, err := a.remoteClient(project)
	if err != nil {
		return err
	}
	return client.Logs(ctx, service, *tail, *follow)
}

func validateServiceArg(service string) error {
	if service == "" {
		return nil
	}
	for _, r := range service {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("[logs:args] Service name contains unsupported characters: %q", service)
	}
	return nil
}

func (a *app) cmdRollback(ctx context.Context) error {
	project, err := a.loadProject()
	if err != nil {
		return err
	}
	client, err := a.remoteClient(project)
	if err != nil {
		return err
	}
	releases, err := client.ListReleases(ctx)
	if err != nil {
		return err
	}
	currentID, err := client.CurrentReleaseID(ctx)
	if err != nil {
		return err
	}
	previous, err := release.SelectPrevious(releases, currentID)
	if err != nil {
		return err
	}

	fmt.Fprintln(a.stdout, "Rolling back")
	fmt.Fprintln(a.stdout, "Current release: "+currentID)
	fmt.Fprintln(a.stdout, "Previous release: "+previous.ID)

	if err := client.ActivateExistingRelease(ctx, previous.ID); err != nil {
		return err
	}
	return client.ComposeUpAndStatus(ctx)
}

func (a *app) cmdPrune(ctx context.Context, args []string) error {
	pruneFlags := flag.NewFlagSet("prune", flag.ContinueOnError)
	pruneFlags.SetOutput(a.stderr)
	yes := pruneFlags.Bool("yes", a.yes, "answer yes to confirmation prompt")
	if err := pruneFlags.Parse(args); err != nil {
		return err
	}
	if len(pruneFlags.Args()) > 0 {
		return fmt.Errorf("[prune:args] Usage: tarsail prune [--yes]")
	}

	project, err := a.loadProject()
	if err != nil {
		return err
	}
	client, err := a.remoteClient(project)
	if err != nil {
		return err
	}
	releases, err := client.ListReleases(ctx)
	if err != nil {
		return err
	}
	currentID, err := client.CurrentReleaseID(ctx)
	if err != nil {
		return err
	}
	candidates, err := release.PruneCandidates(releases, currentID, project.Deploy.KeepReleases)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		fmt.Fprintln(a.stdout, "No old releases to prune.")
		return nil
	}

	fmt.Fprintln(a.stdout, "The following old releases will be deleted:")
	var ids []string
	for _, candidate := range candidates {
		fmt.Fprintln(a.stdout, "  "+candidate.ID)
		ids = append(ids, candidate.ID)
	}
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Current release will be kept:")
	fmt.Fprintln(a.stdout, "  "+currentID)
	fmt.Fprintln(a.stdout)

	if !*yes {
		ok, err := ui.PromptYesNo(a.stdin, a.stdout, "Continue? [y/N] ")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(a.stdout, "Prune cancelled.")
			return nil
		}
	}

	if err := client.RemoveReleases(ctx, ids); err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, "Deleted "+strconv.Itoa(len(ids))+" release(s).")
	return nil
}
