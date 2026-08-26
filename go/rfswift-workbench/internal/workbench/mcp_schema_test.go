package workbench

import (
	"encoding/json"
	"testing"
)

func TestMCPToolSchemasAlwaysUseObjectProperties(t *testing.T) {
	s := &mcpServer{}
	for index, tool := range s.tools() {
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Fatalf("tool %d has no object inputSchema", index)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || properties == nil {
			encoded, _ := json.Marshal(schema)
			t.Fatalf("tool %d properties must be an object: %s", index, encoded)
		}
	}
}
