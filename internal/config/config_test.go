package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "deploy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deploy", "nginx.conf"), []byte("server {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deploy", "prod.env"), []byte("TOKEN=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "tarsail.yml")
	if err := os.WriteFile(configPath, []byte(`project: my-app
target:
  name: prod
  host: example.com
  user: deploy
  port: 22
  path: /opt/my-app
compose:
  file: compose.yaml
  env_file:
    source: deploy/prod.env
    target: shared/.env
build:
  steps:
    - name: Build static assets
      run: npm run build
      dir: deploy
deploy:
  keep_releases: 3
files:
  - source: deploy/nginx.conf
    target: files/nginx/default.conf
secrets:
  - source: deploy/prod.env
    target: shared/prod.env
    mode: 600
`), 0o644); err != nil {
		t.Fatal(err)
	}

	project, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if project.Project != "my-app" {
		t.Fatalf("project name = %q", project.Project)
	}
	if project.ComposePath() != filepath.Join(dir, "compose.yaml") {
		t.Fatalf("ComposePath = %q", project.ComposePath())
	}
	if project.ComposeEnvFileSourcePath() != filepath.Join(dir, "deploy", "prod.env") {
		t.Fatalf("ComposeEnvFileSourcePath = %q", project.ComposeEnvFileSourcePath())
	}
	if len(project.Build.Steps) != 1 || project.Build.Steps[0].Run != "npm run build" {
		t.Fatalf("build steps not loaded: %#v", project.Build.Steps)
	}
	if project.BuildStepDir(project.Build.Steps[0]) != filepath.Join(dir, "deploy") {
		t.Fatalf("BuildStepDir = %q", project.BuildStepDir(project.Build.Steps[0]))
	}
	if len(project.Files) != 1 || project.Files[0].Target != "files/nginx/default.conf" {
		t.Fatalf("files not loaded: %#v", project.Files)
	}
	if len(project.SecretUploads()) != 2 {
		t.Fatalf("SecretUploads length = %d", len(project.SecretUploads()))
	}
}

func TestLoadAllowsGeneratedManagedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "tarsail.yml")
	if err := os.WriteFile(configPath, []byte(`project: my-app
target:
  name: prod
  host: example.com
  user: deploy
  path: /opt/my-app
compose:
  file: compose.yaml
build:
  steps:
    - name: Build web
      run: npm run build:web
files:
  - source: apps/web/dist
    target: files/web-dist
`), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	missing, err := project.MissingManagedFileSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "apps/web/dist" {
		t.Fatalf("MissingManagedFileSources = %#v", missing)
	}
}

func TestLoadRejectsFutureSections(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "tarsail.yml")
	if err := os.WriteFile(configPath, []byte(`project: my-app
targets:
  prod: {}
target:
  name: prod
  host: example.com
  user: deploy
  path: /opt/my-app
compose:
  file: compose.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Unsupported Phase 0 config section") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTargetPath(t *testing.T) {
	valid := []string{"/opt/my-app", "/srv/apps/demo_1"}
	for _, value := range valid {
		if err := ValidateTargetPath(value); err != nil {
			t.Fatalf("ValidateTargetPath(%q) returned error: %v", value, err)
		}
	}

	invalid := []string{"", "relative", "/", "/opt", "/var", "/tmp", "/opt/../app", "/opt/my app", "/opt/app;rm"}
	for _, value := range invalid {
		if err := ValidateTargetPath(value); err == nil {
			t.Fatalf("ValidateTargetPath(%q) returned nil", value)
		}
	}
}

func TestValidateComposeFileRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := ValidateComposeFile(dir, "../compose.yaml"); err == nil {
		t.Fatal("expected traversal error")
	}
	if err := ValidateComposeFile(dir, "/tmp/compose.yaml"); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestLoadRejectsUnsafeManagedPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.txt"), []byte("asset\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "tarsail.yml")
	if err := os.WriteFile(configPath, []byte(`project: my-app
target:
  name: prod
  host: example.com
  user: deploy
  path: /opt/my-app
compose:
  file: compose.yaml
files:
  - source: asset.txt
    target: ../asset.txt
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("expected unsafe file target error")
	}
}

func TestLoadAllowsRemoteOnlyComposeEnvFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "tarsail.yml")
	if err := os.WriteFile(configPath, []byte(`project: my-app
target:
  name: prod
  host: example.com
  user: deploy
  path: /opt/my-app
compose:
  file: compose.yaml
  env_file:
    target: shared/.env
`), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(project.RequiredSharedTargets()) != 1 || project.RequiredSharedTargets()[0] != "shared/.env" {
		t.Fatalf("RequiredSharedTargets = %#v", project.RequiredSharedTargets())
	}
	if len(project.SecretUploads()) != 0 {
		t.Fatalf("SecretUploads = %#v", project.SecretUploads())
	}
}

func TestInitDetectsComposeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "tarsail.yml")
	if err := Init(configPath); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "file: docker-compose.yml") {
		t.Fatalf("generated config did not use detected compose file:\n%s", data)
	}
}
