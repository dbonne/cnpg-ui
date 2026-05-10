package openapi_test

import (
	"encoding/json"
	"testing"

	openapipkg "github.com/dbonne/cnpg-ui/internal/openapi"
)

// TestSpecYAML_NotEmpty verifies the embedded YAML spec is non-empty.
func TestSpecYAML_NotEmpty(t *testing.T) {
	if len(openapipkg.SpecYAML) == 0 {
		t.Fatal("SpecYAML is empty")
	}
}

// TestSpecJSON_IsValidJSON verifies SpecJSON returns valid JSON.
func TestSpecJSON_IsValidJSON(t *testing.T) {
	j := openapipkg.SpecJSON()
	if len(j) == 0 {
		t.Fatal("SpecJSON returned empty bytes")
	}

	var v interface{}
	if err := json.Unmarshal(j, &v); err != nil {
		t.Fatalf("SpecJSON is not valid JSON: %v", err)
	}
}

// TestSpecJSON_ContainsOpenAPIKey verifies the JSON has the expected top-level openapi key.
func TestSpecJSON_ContainsOpenAPIKey(t *testing.T) {
	j := openapipkg.SpecJSON()

	var m map[string]interface{}
	if err := json.Unmarshal(j, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := m["openapi"]; !ok {
		t.Error("SpecJSON missing top-level \"openapi\" key")
	}
}
