package tools

import (
	"fmt"
	"sort"

	"slimebot/internal/domain"
)

const TodoUpdateFunctionName = "todo_update"

// ModelFunctionName returns the model-facing function name for a built-in tool command.
func ModelFunctionName(toolName, command string) string {
	if toolName == "todo" && command == "update" {
		return TodoUpdateFunctionName
	}
	return toolName + "__" + command
}

// BuildRegistryToolDefs builds function-calling tool definitions from the global registry.
func BuildRegistryToolDefs() []domain.ToolDef {
	var defs []domain.ToolDef
	for _, t := range All() {
		if IsStableNameTool(t.Name()) {
			continue
		}
		for _, cmd := range t.Commands() {
			properties := make(map[string]any)
			var required []string
			for _, p := range cmd.Params {
				prop := map[string]any{}
				if schema, ok := p.Schema.(map[string]any); ok && len(schema) > 0 {
					for k, v := range schema {
						prop[k] = v
					}
				} else {
					prop["type"] = "string"
				}
				prop["description"] = p.Description
				if p.Example != "" {
					prop["example"] = p.Example
				}
				properties[p.Name] = prop
				if p.Required {
					required = append(required, p.Name)
				}
			}

			params := map[string]any{
				"type":       "object",
				"properties": properties,
			}
			if len(required) > 0 {
				params["required"] = required
			}

			defs = append(defs, domain.ToolDef{
				Name:        ModelFunctionName(t.Name(), cmd.Name),
				Description: fmt.Sprintf("[%s] %s", t.Name(), cmd.Description),
				Parameters:  params,
			})
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Name == defs[j].Name {
			return defs[i].Description < defs[j].Description
		}
		return defs[i].Name < defs[j].Name
	})
	return defs
}
