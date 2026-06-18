package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/plystra/tarsail/internal/compose"
	"github.com/plystra/tarsail/internal/config"
	localdocker "github.com/plystra/tarsail/internal/docker"
)

type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	Project       string          `json:"project"`
	ReleaseID     string          `json:"release_id"`
	CreatedAt     string          `json:"created_at"`
	CreatedBy     string          `json:"created_by"`
	ComposeFile   string          `json:"compose_file"`
	Images        []ManifestImage `json:"images"`
}

type ManifestImage struct {
	Service string `json:"service"`
	Image   string `json:"image"`
	File    string `json:"file"`
}

type Result struct {
	Path     string
	Manifest Manifest
}

func Create(ctx context.Context, project *config.Project, releaseID string, createdAt time.Time, images []compose.ServiceImage, runner localdocker.Runner) (Result, error) {
	workspace, err := os.MkdirTemp("", "tarsail-bundle-*")
	if err != nil {
		return Result{}, fmt.Errorf("[bundle:create] Could not create temporary bundle workspace: %w", err)
	}
	defer os.RemoveAll(workspace)

	if err := copyFile(project.ComposePath(), filepath.Join(workspace, "compose.yaml")); err != nil {
		return Result{}, err
	}
	if err := os.Mkdir(filepath.Join(workspace, "images"), 0o755); err != nil {
		return Result{}, fmt.Errorf("[bundle:create] Could not create images directory: %w", err)
	}

	manifest := Manifest{
		SchemaVersion: 1,
		Project:       project.Project,
		ReleaseID:     releaseID,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339),
		CreatedBy:     "tarsail",
		ComposeFile:   "compose.yaml",
	}

	for _, image := range images {
		if err := validateRelativeBundlePath(image.File); err != nil {
			return Result{}, err
		}
		outputPath := filepath.Join(workspace, filepath.FromSlash(image.File))
		if err := runner.ImageSave(ctx, image.Image, outputPath); err != nil {
			return Result{}, err
		}
		manifest.Images = append(manifest.Images, ManifestImage{
			Service: image.Service,
			Image:   image.Image,
			File:    image.File,
		})
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("[bundle:manifest] Could not encode manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(filepath.Join(workspace, "manifest.json"), manifestData, 0o644); err != nil {
		return Result{}, fmt.Errorf("[bundle:manifest] Could not write manifest: %w", err)
	}

	bundlePath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.tarsail.tar.gz", project.Project, releaseID))
	if err := archiveDirectory(workspace, bundlePath); err != nil {
		return Result{}, err
	}

	return Result{Path: bundlePath, Manifest: manifest}, nil
}

func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("[bundle:manifest] Unsupported manifest schema_version: %d", manifest.SchemaVersion)
	}
	if err := config.ValidateProjectName(manifest.Project); err != nil {
		return err
	}
	if manifest.ReleaseID == "" || manifest.CreatedAt == "" || manifest.ComposeFile != "compose.yaml" {
		return fmt.Errorf("[bundle:manifest] Manifest is missing required release metadata.")
	}
	for _, image := range manifest.Images {
		if image.Service == "" || image.Image == "" {
			return fmt.Errorf("[bundle:manifest] Manifest image entry is missing service or image.")
		}
		if err := validateRelativeBundlePath(image.File); err != nil {
			return err
		}
	}
	return nil
}

func validateRelativeBundlePath(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("[bundle:path] Bundle path must be relative and use forward slashes: %q", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("[bundle:path] Bundle path contains an unsafe component: %q", value)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("[bundle:create] Could not open Compose file: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("[bundle:create] Could not create bundled Compose file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("[bundle:create] Could not copy Compose file: %w", err)
	}
	return nil
}

func archiveDirectory(srcDir, dstPath string) error {
	out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("[bundle:create] Could not create archive: %w", err)
	}
	defer out.Close()

	gzipWriter := gzip.NewWriter(out)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.WalkDir(srcDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := validateRelativeBundlePath(rel); err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarWriter, file)
		return err
	})
}
