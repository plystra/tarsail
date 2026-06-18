package compose

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServiceImage struct {
	Service string
	Image   string
	File    string
}

var serviceNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func DiscoverServiceImages(configOutput string) ([]ServiceImage, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(configOutput), &root); err != nil {
		return nil, fmt.Errorf("[compose:config] Could not parse Docker Compose config output: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("[compose:config] Docker Compose config output was not a YAML mapping.")
	}

	services := mappingValue(root.Content[0], "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("[compose:config] Compose config has no services mapping.")
	}

	var result []ServiceImage
	for i := 0; i < len(services.Content); i += 2 {
		name := services.Content[i].Value
		body := services.Content[i+1]
		if !serviceNameRe.MatchString(name) {
			return nil, fmt.Errorf("[compose:service] Service name is unsafe for bundle filenames: %q\n\nHow to fix:\n  Use only letters, numbers, dot, underscore, or hyphen in service names.", name)
		}
		if body.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("[compose:service] Service %q must be a mapping.", name)
		}
		imageNode := mappingValue(body, "image")
		buildNode := mappingValue(body, "build")
		image := ""
		if imageNode != nil && imageNode.Kind == yaml.ScalarNode {
			image = strings.TrimSpace(imageNode.Value)
		}
		if buildNode != nil && image == "" {
			return nil, fmt.Errorf("[compose:image] Service %q has a build section but no explicit image tag.\n\nWhy it matters:\n  Tarsail needs stable image tags to save and load images.\n\nHow to fix:\n  Add an image field:\n    services:\n      %s:\n        image: my-app-%s:local", name, name, name)
		}
		if image == "" {
			return nil, fmt.Errorf("[compose:image] Service %q has no image tag.\n\nWhy it matters:\n  Tarsail must know which image to save into the release bundle.\n\nHow to fix:\n  Add an explicit image field to the service.", name)
		}
		result = append(result, ServiceImage{
			Service: name,
			Image:   image,
			File:    "images/" + name + ".tar",
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Service < result[j].Service
	})
	return result, nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
