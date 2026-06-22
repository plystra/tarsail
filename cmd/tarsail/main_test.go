package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plystra/tarsail/internal/config"
)

func TestRemoteClientPromptsForPasswordOnce(t *testing.T) {
	project := testProject(t)
	var stdout bytes.Buffer
	application := app{
		stdin:       bytes.NewBufferString("secret-password\n"),
		stdout:      &stdout,
		stderr:      &bytes.Buffer{},
		askPassword: true,
	}

	first, err := application.remoteClient(project)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.remoteClient(project)
	if err != nil {
		t.Fatal(err)
	}

	if first.Auth.Password != "secret-password" || second.Auth.Password != "secret-password" {
		t.Fatalf("password not reused: %#v %#v", first.Auth, second.Auth)
	}
	if got := strings.Count(stdout.String(), "SSH password for deploy@example.com: "); got != 1 {
		t.Fatalf("password prompt count = %d, output = %q", got, stdout.String())
	}
}

func TestRemoteClientRejectsIdentityFileAndPasswordTogether(t *testing.T) {
	project := testProject(t)
	application := app{
		stdin:        bytes.NewBufferString("secret-password\n"),
		stdout:       &bytes.Buffer{},
		stderr:       &bytes.Buffer{},
		identityFile: "id_ed25519",
		askPassword:  true,
	}

	if _, err := application.remoteClient(project); err == nil {
		t.Fatal("expected auth mode conflict error")
	}
}

func TestRemoteClientResolvesIdentityFile(t *testing.T) {
	project := testProject(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0o600); err != nil {
		t.Fatal(err)
	}
	application := app{
		stdin:        &bytes.Buffer{},
		stdout:       &bytes.Buffer{},
		stderr:       &bytes.Buffer{},
		identityFile: keyPath,
	}

	client, err := application.remoteClient(project)
	if err != nil {
		t.Fatal(err)
	}
	if client.Auth.IdentityFile != keyPath {
		t.Fatalf("identity file = %q, want %q", client.Auth.IdentityFile, keyPath)
	}
}

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	application := app{
		stdin:  &bytes.Buffer{},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	if err := application.run(t.Context(), []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "tarsail dev" {
		t.Fatalf("version output = %q", got)
	}
}

func TestVersionFlag(t *testing.T) {
	var stdout bytes.Buffer
	application := app{
		stdin:  &bytes.Buffer{},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	if err := application.run(t.Context(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "tarsail dev" {
		t.Fatalf("version output = %q", got)
	}
}

func TestVersionShortFlag(t *testing.T) {
	var stdout bytes.Buffer
	application := app{
		stdin:  &bytes.Buffer{},
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	if err := application.run(t.Context(), []string{"-v"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "tarsail dev" {
		t.Fatalf("version output = %q", got)
	}
}

func testProject(t *testing.T) *config.Project {
	t.Helper()
	return &config.Project{
		File: config.File{
			Project: "my-app",
			Target: config.Target{
				Name: "prod",
				Host: "example.com",
				User: "deploy",
				Port: 22,
				Path: "/opt/my-app",
			},
			Compose: config.Compose{File: "compose.yaml"},
		},
		Root: t.TempDir(),
	}
}
