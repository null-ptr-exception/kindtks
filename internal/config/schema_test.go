package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testProfileSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "images": {"type": "object", "additionalProperties": false,
      "properties": {"cilium": {"type": "string"}}},
    "gateway": {"type": "object", "additionalProperties": false,
      "properties": {"extraHosts": {"type": "array", "items": {"type": "string", "pattern": "^[a-z*.]+$"}}}}
  }
}`

func obj(t *testing.T, yaml string) map[string]any {
	t.Helper()
	doc, err := LoadDocument(writeFile(t, "d.yaml", yaml))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestValidateCommon_Valid(t *testing.T) {
	doc := obj(t, `registries:
  quay.io:
    hosts: "server = 'x'"
    auth: {username: u, password: p}
profiles:
  gen1:
    images: {cilium: "quay.io/cilium/cilium:v1"}
    anything: [1, 2]
`)
	if err := ValidateCommon(doc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommon_Errors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"unknown top-level key": {"bogus: 1\n", "bogus"},
		"extra registry field":  {"registries:\n  quay.io:\n    mirror: x\n", "registries.quay.io"},
		"non-string image":      {"profiles:\n  gen1:\n    images: {cilium: 3}\n", "profiles.gen1.images.cilium"},
		"profile not an object": {"profiles:\n  gen1: [a]\n", "profiles.gen1"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateCommon(obj(t, c.yaml))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error mentioning %q, got %v", c.want, err)
			}
		})
	}
}

func TestValidateProfile_ReportsPath(t *testing.T) {
	section := obj(t, "gateway:\n  extraHosts: [\"BAD\"]\n")
	err := ValidateProfile("gen1", []byte(testProfileSchema), section)
	if err == nil || !strings.Contains(err.Error(), "profiles.gen1.gateway.extraHosts[0]") {
		t.Fatalf("want path profiles.gen1.gateway.extraHosts[0], got %v", err)
	}
}

func TestValidateProfile_UnknownOption(t *testing.T) {
	err := ValidateProfile("gen1", []byte(testProfileSchema), obj(t, "bogus: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("want error mentioning bogus, got %v", err)
	}
}

func TestProfileSchema_DefaultAllowsOnlyImages(t *testing.T) {
	schema, err := ProfileSchema(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProfile("plain", schema, obj(t, "images: {x: \"img:1\"}\n")); err != nil {
		t.Errorf("images should be allowed: %v", err)
	}
	if err := ValidateProfile("plain", schema, obj(t, "gateway: {}\n")); err == nil {
		t.Error("non-images key should be rejected")
	}
}

func TestProfileSchema_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.schema.json"), []byte(testProfileSchema), 0644)
	schema, err := ProfileSchema(dir)
	if err != nil || string(schema) != testProfileSchema {
		t.Fatalf("want file contents, got %q, %v", schema, err)
	}
}

func TestValidateProfile_ReportsPathForNumericKey(t *testing.T) {
	numericKeySchema := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "hosts": {"type": "object", "additionalProperties": {
      "type": "object", "additionalProperties": false,
      "properties": {"port": {"type": "integer"}}
    }}
  }
}`
	section := obj(t, "hosts:\n  \"5000\":\n    port: \"bad\"\n")
	err := ValidateProfile("gen1", []byte(numericKeySchema), section)
	if err == nil || !strings.Contains(err.Error(), "profiles.gen1.hosts.5000.port") {
		t.Fatalf("want path profiles.gen1.hosts.5000.port, got %v", err)
	}
}

func TestCombinedSchema_ResolvesRootRelativeRefs(t *testing.T) {
	refSchema := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "gateway": {"$ref": "#/$defs/gateway"}
  },
  "$defs": {
    "gateway": {"type": "object", "additionalProperties": false,
      "properties": {"port": {"type": "integer"}}}
  }
}`
	combined, err := CombinedSchema("gen1", []byte(refSchema))
	if err != nil {
		t.Fatal(err)
	}
	data, err := jsonMarshal(combined)
	if err != nil {
		t.Fatal(err)
	}
	ok := obj(t, "profiles:\n  gen1:\n    gateway: {port: 8080}\n")
	if err := validate("combined.json", data, ok, ""); err != nil {
		t.Errorf("valid doc rejected: %v", err)
	}
	bad := obj(t, "profiles:\n  gen1:\n    gateway: {port: \"bad\"}\n")
	if err := validate("combined.json", data, bad, ""); err == nil {
		t.Error("$ref/$defs in the profile schema should still be enforced through the combined schema")
	}
}

func TestCombinedSchema(t *testing.T) {
	combined, err := CombinedSchema("gen1", []byte(testProfileSchema))
	if err != nil {
		t.Fatal(err)
	}
	data, err := jsonMarshal(combined)
	if err != nil {
		t.Fatal(err)
	}
	ok := obj(t, "profiles:\n  gen1:\n    gateway: {extraHosts: [\"*.example.net\"]}\n")
	if err := validate("combined.json", data, ok, ""); err != nil {
		t.Errorf("valid doc rejected: %v", err)
	}
	bad := obj(t, "profiles:\n  gen1:\n    bogus: 1\n")
	if err := validate("combined.json", data, bad, ""); err == nil {
		t.Error("combined schema should apply the profile schema at profiles.gen1")
	}
}
