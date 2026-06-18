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
deploy:
  keep_releases: 3
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
