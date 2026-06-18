package config

import (
	"fmt"
	"os"
	"path/filepath"
)

var composeCandidates = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

func Init(configPath string) error {
	if configPath == "" {
		configPath = DefaultFile
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("[init:config] Could not resolve config path: %w", err)
	}
	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("[init:config] Config file already exists: %s\n\nHow to fix:\n  Edit the existing file or remove it before running init again.", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("[init:config] Could not inspect config path: %w", err)
	}

	root := filepath.Dir(absPath)
	composeFile := detectComposeFile(root)
	content := fmt.Sprintf(`project: my-app

target:
  name: prod
  host: example.com
  user: deploy
  port: 22
  path: /opt/my-app

compose:
  file: %s

deploy:
  keep_releases: 3
`, composeFile)

	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("[init:config] Could not write config file: %w", err)
	}
	return nil
}

func detectComposeFile(root string) string {
	for _, candidate := range composeCandidates {
		if info, err := os.Stat(filepath.Join(root, candidate)); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "compose.yaml"
}
