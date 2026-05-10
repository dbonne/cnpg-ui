// Package openapi embeds the OpenAPI specification file and exposes it for
// serving via HTTP. The spec is embedded at build time from api/openapi.yaml
// (relative to the repository root, two directories above this package).
package openapi

import (
	_ "embed"

	sigsyaml "sigs.k8s.io/yaml"
)

// SpecYAML holds the raw bytes of api/openapi.yaml, embedded at build time.
// It is served as-is with Content-Type: application/x-yaml.
//
//go:embed openapi.yaml
var SpecYAML []byte

// specJSON holds the JSON-encoded version of SpecYAML, computed once at init.
var specJSON []byte

func init() {
	j, err := sigsyaml.YAMLToJSON(SpecYAML)
	if err != nil {
		// Panic at startup: a malformed embedded spec is a build-time error.
		panic("openapi: failed to convert YAML spec to JSON: " + err.Error())
	}
	specJSON = j
}

// SpecJSON returns the OpenAPI specification as a JSON byte slice.
// The conversion from the embedded YAML is performed once at program startup.
func SpecJSON() []byte {
	return specJSON
}
