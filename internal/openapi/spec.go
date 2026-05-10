// Package openapi embeds the OpenAPI specification file and exposes it for
// serving via HTTP. The spec is embedded at build time from api/openapi.yaml
// (relative to the repository root, two directories above this package).
package openapi

import (
	_ "embed"
)

// SpecYAML holds the raw bytes of api/openapi.yaml, embedded at build time.
// It is served as-is with Content-Type: application/x-yaml.
//
//go:embed openapi.yaml
var SpecYAML []byte
