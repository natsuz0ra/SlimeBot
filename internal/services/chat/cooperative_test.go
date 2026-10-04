package chat

import (
	"encoding/json"
	"testing"
)

func TestCooperativeDefinitionsHaveValidRequiredArrays(t *testing.T) {
	for _, def := range cooperativeDefs() {
		t.Run(def.Name, func(t *testing.T) {
			data, err := json.Marshal(def.Parameters)
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatal(err)
			}
			required, ok := schema["required"].([]any)
			if !ok {
				t.Fatalf("required must serialize as an array, got %s", data)
			}
			properties := schema["properties"].(map[string]any)
			for _, field := range required {
				name, ok := field.(string)
				if !ok || properties[name] == nil {
					t.Fatalf("invalid required field: %v", field)
				}
			}
		})
	}
}
