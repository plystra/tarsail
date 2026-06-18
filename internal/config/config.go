package config

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultFile         = "tarsail.yml"
	DefaultPort         = 22
	DefaultKeepReleases = 3
)

type File struct {
	Project string  `yaml:"project"`
	Target  Target  `yaml:"target"`
	Compose Compose `yaml:"compose"`
	Deploy  Deploy  `yaml:"deploy"`
}

type Target struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Port int    `yaml:"port"`
	Path string `yaml:"path"`
}

type Compose struct {
	File string `yaml:"file"`
}

type Deploy struct {
	KeepReleases int `yaml:"keep_releases"`
}

type Project struct {
	File
	ConfigPath string
	Root       string
}

var (
	projectNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	targetNameRe  = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	targetHostRe  = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
	targetUserRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
	targetPathRe  = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
)

var supportedTopLevelKeys = map[string]struct{}{
	"project": {},
	"target":  {},
	"compose": {},
	"deploy":  {},
}

var unsupportedFutureTopLevelKeys = map[string]struct{}{
	"backups":       {},
	"builder":       {},
	"builders":      {},
	"caddy":         {},
	"ci":            {},
	"kubernetes":    {},
	"nginx":         {},
	"notifications": {},
	"plugins":       {},
	"registries":    {},
	"registry":      {},
	"secrets":       {},
	"servers":       {},
	"swarm":         {},
	"targets":       {},
	"tls":           {},
}

func Load(configPath string) (*Project, error) {
	if configPath == "" {
		configPath = DefaultFile
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("[config:load] Could not resolve config path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("[config:load] Config file not found: %s\n\nHow to fix:\n  Run \"tarsail init\" or pass --config <path>.", configPath)
		}
		return nil, fmt.Errorf("[config:load] Could not read config file: %w", err)
	}

	if err := validateTopLevelKeys(data); err != nil {
		return nil, err
	}

	var file File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("[config:parse] Invalid tarsail.yml.\n\nWhy it matters:\n  Tarsail needs an exact Phase 0 config shape.\n\nHow to fix:\n  Check YAML syntax and remove unsupported fields.\n\nDetails:\n  %w", err)
	}

	applyDefaults(&file)

	project := &Project{
		File:       file,
		ConfigPath: absPath,
		Root:       filepath.Dir(absPath),
	}
	if err := project.Validate(); err != nil {
		return nil, err
	}

	return project, nil
}

func applyDefaults(file *File) {
	if file.Target.Port == 0 {
		file.Target.Port = DefaultPort
	}
	if file.Deploy.KeepReleases == 0 {
		file.Deploy.KeepReleases = DefaultKeepReleases
	}
}

func validateTopLevelKeys(data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("[config:parse] Invalid YAML: %w", err)
	}
	if len(node.Content) == 0 {
		return fmt.Errorf("[config:parse] Config file is empty.\n\nHow to fix:\n  Run \"tarsail init\" to generate a minimal config.")
	}
	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("[config:parse] Config root must be a YAML mapping.")
	}

	var unknown []string
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i].Value
		if _, ok := supportedTopLevelKeys[key]; ok {
			continue
		}
		if _, future := unsupportedFutureTopLevelKeys[key]; future {
			return fmt.Errorf("[config:scope] Unsupported Phase 0 config section: %s\n\nWhy it matters:\n  Phase 0 supports exactly one target and no CI, registry, secrets, Kubernetes, or plugin sections.\n\nHow to fix:\n  Remove the %q section from tarsail.yml.", key, key)
		}
		unknown = append(unknown, key)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("[config:parse] Unknown top-level config key: %s\n\nHow to fix:\n  Supported Phase 0 keys are project, target, compose, and deploy.", strings.Join(unknown, ", "))
	}
	return nil
}

func (p *Project) Validate() error {
	if err := ValidateProjectName(p.Project); err != nil {
		return err
	}
	if err := ValidateTarget(p.Target); err != nil {
		return err
	}
	if err := ValidateComposeFile(p.Root, p.Compose.File); err != nil {
		return err
	}
	if p.Deploy.KeepReleases < 1 || p.Deploy.KeepReleases > 20 {
		return fmt.Errorf("[config:deploy] deploy.keep_releases must be between 1 and 20.\n\nHow to fix:\n  Set deploy.keep_releases to a small positive number such as 3.")
	}
	return nil
}

func ValidateProjectName(name string) error {
	if !projectNameRe.MatchString(name) {
		return fmt.Errorf("[config:project] Invalid project name: %q\n\nWhy it matters:\n  Tarsail uses the project name in filenames, Compose project names, and release metadata.\n\nHow to fix:\n  Use lowercase letters, numbers, hyphen, or underscore only.", name)
	}
	return nil
}

func ValidateTarget(target Target) error {
	if !targetNameRe.MatchString(target.Name) {
		return fmt.Errorf("[config:target] target.name must use lowercase letters, numbers, hyphen, or underscore.")
	}
	if !targetHostRe.MatchString(target.Host) {
		return fmt.Errorf("[config:target] target.host is missing or contains unsupported characters.\n\nHow to fix:\n  Use a hostname such as example.com or an IPv4 address such as 192.0.2.10.")
	}
	if !targetUserRe.MatchString(target.User) {
		return fmt.Errorf("[config:target] target.user is missing or unsafe for SSH command construction.\n\nHow to fix:\n  Use a simple SSH username such as deploy.")
	}
	if target.Port < 1 || target.Port > 65535 {
		return fmt.Errorf("[config:target] target.port must be between 1 and 65535.")
	}
	if err := ValidateTargetPath(target.Path); err != nil {
		return err
	}
	return nil
}

func ValidateTargetPath(targetPath string) error {
	if targetPath == "" || !strings.HasPrefix(targetPath, "/") {
		return fmt.Errorf("[config:target] target.path must be an absolute remote path.\n\nHow to fix:\n  Use a project-specific path such as /opt/my-app.")
	}
	if strings.ContainsRune(targetPath, 0) || !targetPathRe.MatchString(targetPath) {
		return fmt.Errorf("[config:target] target.path contains unsupported characters.\n\nWhy it matters:\n  Tarsail passes this path to remote shell commands.\n\nHow to fix:\n  Use a simple absolute path such as /opt/my-app.")
	}
	clean := path.Clean(targetPath)
	if clean != targetPath {
		return fmt.Errorf("[config:target] target.path must be clean: %s\n\nHow to fix:\n  Use %s instead.", targetPath, clean)
	}
	if clean == "/" {
		return fmt.Errorf("[config:target] target.path must not be /.\n\nHow to fix:\n  Use a project-specific path such as /opt/my-app.")
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return fmt.Errorf("[config:target] target.path must not contain '..'.")
		}
	}
	switch clean {
	case "/opt", "/var", "/usr", "/home", "/etc", "/root", "/tmp":
		return fmt.Errorf("[config:target] target.path is too broad: %s\n\nWhy it matters:\n  Tarsail may manage subdirectories under this path.\n\nHow to fix:\n  Use a project-specific path such as %s/my-app.", clean, clean)
	}
	return nil
}

func ValidateComposeFile(root, composeFile string) error {
	if composeFile == "" {
		return fmt.Errorf("[config:compose] compose.file is required.")
	}
	if filepath.IsAbs(composeFile) || path.IsAbs(composeFile) {
		return fmt.Errorf("[config:compose] compose.file must be relative to the project root.")
	}
	if strings.Contains(composeFile, "\\") {
		return fmt.Errorf("[config:compose] compose.file must use forward slashes, not backslashes.")
	}
	if hasUnsafePathComponent(composeFile) {
		return fmt.Errorf("[config:compose] compose.file must not contain '..' or empty path components.")
	}
	if strings.ContainsAny(composeFile, "\x00;&|$><`'\"(){}[]*?~!\n\r\t ") {
		return fmt.Errorf("[config:compose] compose.file contains unsupported characters.")
	}
	fullPath := filepath.Join(root, filepath.FromSlash(composeFile))
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("[config:compose] Compose file not found: %s\n\nHow to fix:\n  Create the file or update compose.file in tarsail.yml.", composeFile)
		}
		return fmt.Errorf("[config:compose] Could not inspect Compose file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("[config:compose] compose.file points to a directory: %s", composeFile)
	}
	return nil
}

func hasUnsafePathComponent(value string) bool {
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return true
		}
	}
	return false
}

func (p *Project) ComposePath() string {
	return filepath.Join(p.Root, filepath.FromSlash(p.Compose.File))
}
