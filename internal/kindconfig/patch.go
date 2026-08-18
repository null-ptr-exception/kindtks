package kindconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rophy/kindtks/internal/config"
	"gopkg.in/yaml.v3"
)

type kindCluster struct {
	ContainerdConfigPatches []string `yaml:"containerdConfigPatches,omitempty"`
}

// PatchForRegistries reads a kind-config YAML file and, if registries or auth
// entries are present, appends a containerdConfigPatches entry for insecure
// HTTP access and/or registry authentication.
// Returns the path to use — the original file when no patch is needed, or a
// temp file with the patched config.
func PatchForRegistries(kindConfigPath string, registries []string, auth map[string]*config.RegistryAuth) (string, func(), error) {
	noop := func() {}

	if len(registries) == 0 && len(auth) == 0 {
		return kindConfigPath, noop, nil
	}

	data, err := os.ReadFile(kindConfigPath)
	if err != nil {
		return "", noop, fmt.Errorf("reading kind config: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", noop, fmt.Errorf("parsing kind config: %w", err)
	}

	patch := config.ContainerdPatch(registries, auth)

	// Find or create containerdConfigPatches in the YAML
	root := doc.Content[0] // mapping node
	var patchesNode *yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "containerdConfigPatches" {
			patchesNode = root.Content[i+1]
			break
		}
	}

	patchValue := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Style: yaml.LiteralStyle,
		Value: patch,
	}

	if patchesNode != nil {
		patchesNode.Content = append(patchesNode.Content, patchValue)
	} else {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "containerdConfigPatches"},
			&yaml.Node{
				Kind:    yaml.SequenceNode,
				Content: []*yaml.Node{patchValue},
			},
		)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", noop, fmt.Errorf("marshalling patched kind config: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "kindtks-*")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	tmpFile := filepath.Join(tmpDir, "kind-config.yaml")
	if err := os.WriteFile(tmpFile, out, 0644); err != nil {
		cleanup()
		return "", noop, err
	}

	return tmpFile, cleanup, nil
}
