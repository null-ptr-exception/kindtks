package kindconfig

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ContainerCertsDir is where containerd reads per-registry hosts.toml files
// inside each Kind node.
const ContainerCertsDir = "/etc/containerd/certs.d"

const configPathPatch = `[plugins."io.containerd.grpc.v1.cri".registry]
  config_path = "/etc/containerd/certs.d"`

// Options describes the registry changes to apply to a kind config.
type Options struct {
	// CertsDir is a host directory of <registry>/hosts.toml files, mounted
	// read-only at ContainerCertsDir on every node. Empty means none.
	CertsDir string
	// AuthPatch is an extra containerdConfigPatches entry holding registry
	// credentials. Empty means none.
	AuthPatch string
}

// Patch applies opts to the kind config at kindConfigPath. It returns the
// original path when there is nothing to apply, otherwise a temp file with the
// patched config and a cleanup func that removes it.
func Patch(kindConfigPath string, opts Options) (string, func(), error) {
	noop := func() {}

	if opts.CertsDir == "" && opts.AuthPatch == "" {
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
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", noop, fmt.Errorf("kind config %s is not a YAML mapping", kindConfigPath)
	}
	root := doc.Content[0]

	var patches []string
	if opts.CertsDir != "" {
		if err := addCertsMounts(root, opts.CertsDir); err != nil {
			return "", noop, err
		}
		patches = append(patches, configPathPatch)
	}
	if opts.AuthPatch != "" {
		patches = append(patches, opts.AuthPatch)
	}
	appendPatches(root, patches)

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

// addCertsMounts adds a read-only mount of certsDir at ContainerCertsDir to
// every node, creating kind's default single control-plane node when the
// config has no nodes.
func addCertsMounts(root *yaml.Node, certsDir string) error {
	nodes := mapGet(root, "nodes")
	if nodes == nil {
		cp := &yaml.Node{Kind: yaml.MappingNode}
		mapSet(cp, "role", str("control-plane"))
		nodes = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{cp}}
		mapSet(root, "nodes", nodes)
	}

	for i, n := range nodes.Content {
		if n.Kind != yaml.MappingNode {
			return fmt.Errorf("kind config nodes[%d] is not a mapping", i)
		}
		mounts := mapGet(n, "extraMounts")
		if mounts == nil {
			mounts = &yaml.Node{Kind: yaml.SequenceNode}
			mapSet(n, "extraMounts", mounts)
		}
		for _, m := range mounts.Content {
			if cp := mapGet(m, "containerPath"); cp != nil && path.Clean(cp.Value) == ContainerCertsDir {
				return fmt.Errorf("kind config nodes[%d] already mounts %s", i, ContainerCertsDir)
			}
		}
		mount := &yaml.Node{Kind: yaml.MappingNode}
		mapSet(mount, "hostPath", str(certsDir))
		mapSet(mount, "containerPath", str(ContainerCertsDir))
		mapSet(mount, "readOnly", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		mounts.Content = append(mounts.Content, mount)
	}
	return nil
}

// appendPatches appends entries to containerdConfigPatches, creating it if needed.
func appendPatches(root *yaml.Node, patches []string) {
	if len(patches) == 0 {
		return
	}
	seq := mapGet(root, "containerdConfigPatches")
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode}
		mapSet(root, "containerdConfigPatches", seq)
	}
	for _, p := range patches {
		seq.Content = append(seq.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Style: yaml.LiteralStyle,
			Value: p,
		})
	}
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mapSet appends key: val to mapping m. Callers only use it for absent keys.
func mapSet(m *yaml.Node, key string, val *yaml.Node) {
	m.Content = append(m.Content, str(key), val)
}

func str(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}
