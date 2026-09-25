package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/common.json
var commonSchemaJSON []byte

// defaultProfileSchemaJSON applies to profiles without a config.schema.json:
// only the reserved images map is allowed.
var defaultProfileSchemaJSON = []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "images": {"type": "object", "additionalProperties": {"type": "string"}}
  }
}`)

// ProfileSchema returns the profile's config.schema.json, or the default
// images-only schema when the profile has none.
func ProfileSchema(profileDir string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(profileDir, "config.schema.json"))
	if errors.Is(err, os.ErrNotExist) {
		return defaultProfileSchemaJSON, nil
	}
	return data, err
}

// ValidateCommon validates a whole config document against the common schema.
func ValidateCommon(doc any) error {
	return validate("common.json", commonSchemaJSON, doc, "")
}

// ValidateProfile validates one profiles.<name> section against schemaJSON.
func ValidateProfile(name string, schemaJSON []byte, section any) error {
	return validate(name+".schema.json", schemaJSON, section, "profiles."+name)
}

// CombinedSchema returns the common schema with the profile's schema placed
// at profiles.<name>, for editor completion.
func CombinedSchema(name string, profileSchemaJSON []byte) (map[string]any, error) {
	var common, prof map[string]any
	if err := json.Unmarshal(commonSchemaJSON, &common); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(profileSchemaJSON, &prof); err != nil {
		return nil, fmt.Errorf("parsing %s schema: %w", name, err)
	}
	delete(prof, "$schema")

	profiles := common["properties"].(map[string]any)["profiles"].(map[string]any)
	props, _ := profiles["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		profiles["properties"] = props
	}
	props[name] = prof
	return common, nil
}

func validate(name string, schemaJSON []byte, inst any, prefix string) error {
	url := "mem://" + name
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("parsing schema %s: %w", name, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return fmt.Errorf("loading schema %s: %w", name, err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return fmt.Errorf("compiling schema %s: %w", name, err)
	}

	err = sch.Validate(inst)
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	var msgs []string
	collectLeaves(ve, prefix, message.NewPrinter(language.English), &msgs)
	sort.Strings(msgs)
	return fmt.Errorf("invalid config:\n  %s", strings.Join(msgs, "\n  "))
}

// collectLeaves flattens a validation error tree into one message per leaf.
func collectLeaves(e *jsonschema.ValidationError, prefix string, p *message.Printer, out *[]string) {
	if len(e.Causes) == 0 {
		*out = append(*out, fmt.Sprintf("%s: %s", instancePath(prefix, e.InstanceLocation), e.ErrorKind.LocalizedString(p)))
		return
	}
	for _, c := range e.Causes {
		collectLeaves(c, prefix, p, out)
	}
}

// instancePath renders a location like profiles.gen1.gateway.extraHosts[0].
func instancePath(prefix string, loc []string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, seg := range loc {
		if isIndex(seg) {
			fmt.Fprintf(&b, "[%s]", seg)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	if b.Len() == 0 {
		return "(root)"
	}
	return b.String()
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
