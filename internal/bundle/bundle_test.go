package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/plystra/tarsail/internal/compose"
	"github.com/plystra/tarsail/internal/config"
)

func TestValidateManifest(t *testing.T) {
	manifest := Manifest{
		SchemaVersion: 1,
		Project:       "my-app",
		ReleaseID:     "20260618-120000-a111",
		CreatedAt:     "2026-06-18T12:00:00Z",
		ComposeFile:   "compose.yaml",
		Images: []ManifestImage{
			{Service: "api", Image: "my-app-api:local", File: "images/api.tar"},
		},
	}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest returned error: %v", err)
	}
}

func TestValidateManifestRejectsUnsafeImagePath(t *testing.T) {
	manifest := Manifest{
		SchemaVersion: 1,
		Project:       "my-app",
		ReleaseID:     "20260618-120000-a111",
		CreatedAt:     "2026-06-18T12:00:00Z",
		ComposeFile:   "compose.yaml",
		Images: []ManifestImage{
			{Service: "api", Image: "my-app-api:local", File: "../api.tar"},
		},
	}
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateIncludesConfiguredFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "index.html"), []byte("<html></html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &config.Project{
		File: config.File{
			Project: "my-app",
			Compose: config.Compose{File: "compose.yaml"},
			Files: []config.ManagedFile{
				{Source: "dist", Target: "files/web-dist"},
			},
		},
		Root: root,
	}

	result, err := Create(context.Background(), project, "20260618-120000-a111", time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), []compose.ServiceImage{
		{Service: "api", Image: "my-app-api:local", File: "images/api.tar"},
	}, fakeImageSaver{})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	defer os.Remove(result.Path)

	if len(result.Manifest.Files) != 1 || result.Manifest.Files[0].Target != "files/web-dist" {
		t.Fatalf("manifest files = %#v", result.Manifest.Files)
	}
	entries := archiveEntries(t, result.Path)
	if !entries["files/web-dist/index.html"] {
		t.Fatalf("archive entries missing managed file: %#v", entries)
	}
	if !entries["images/api.tar"] {
		t.Fatalf("archive entries missing image tar: %#v", entries)
	}
}

type fakeImageSaver struct{}

func (fakeImageSaver) ImageSave(_ context.Context, _ string, outputPath string) error {
	return os.WriteFile(outputPath, []byte("image tar\n"), 0o644)
}

func archiveEntries(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	entries := map[string]bool{}
	for {
		header, err := reader.Next()
		if err != nil {
			break
		}
		entries[header.Name] = true
	}
	return entries
}
