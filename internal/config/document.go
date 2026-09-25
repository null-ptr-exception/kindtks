package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// LoadDocument reads a YAML config file into JSON-compatible generic values
// (map[string]any, []any, string, bool, json.Number, nil). A missing or empty
// file yields an empty object.
func LoadDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}

	var raw any
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if raw == nil {
		return map[string]any{}, nil
	}

	doc, err := normalize(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parsing %s: top level must be a mapping", path)
	}
	return obj, nil
}

// normalize converts decoded YAML into the JSON value model the schema
// validator expects by round-tripping it through JSON.
func normalize(v any) (any, error) {
	v, err := stringifyKeys(v)
	if err != nil {
		return nil, err
	}
	data, err := jsonMarshal(v)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(data))
}

// stringifyKeys walks decoded YAML looking for mapping nodes with non-string
// keys (map[any]any, as produced by yaml.v3 for those), which json.Marshal
// cannot handle, and reports a clear error naming the offending key.
func stringifyKeys(v any) (any, error) {
	switch m := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			s, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("mapping key %v is not a string; quote it", k)
			}
			nv, err := stringifyKeys(val)
			if err != nil {
				return nil, err
			}
			out[s] = nv
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			nv, err := stringifyKeys(val)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case []any:
		out := make([]any, len(m))
		for i, val := range m {
			nv, err := stringifyKeys(val)
			if err != nil {
				return nil, err
			}
			out[i] = nv
		}
		return out, nil
	default:
		return v, nil
	}
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
