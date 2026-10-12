package domain

import (
	"bytes"
	"encoding/json"
	"io"
)

type AssistantToolDefinition struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
}

// AssistantToolsV1 is the single source for public tool names and argument schemas.
func AssistantToolsV1() []AssistantToolDefinition {
	str := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	definition := func(name string, props map[string]any, required []string) AssistantToolDefinition {
		return AssistantToolDefinition{name, map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}
	}
	return []AssistantToolDefinition{
		definition("list_files", map[string]any{"path": str}, []string{}),
		definition("read_file", map[string]any{"path": str, "offset": integer, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 65536}}, []string{"path"}),
		definition("search", map[string]any{"pattern": str, "path": str}, []string{"pattern"}),
		definition("write_file", map[string]any{"path": str, "content": str, "executable": boolean}, []string{"path", "content"}),
		definition("delete_file", map[string]any{"path": str}, []string{"path"}),
		definition("search_packages", map[string]any{"query": str}, []string{"query"}),
		definition("evaluate", map[string]any{}, []string{}),
		definition("build", map[string]any{}, []string{}),
		definition("lab_status", map[string]any{}, []string{}),
		definition("prepare_npm", map[string]any{"package": str, "version": str, "id": str}, []string{"package", "id"}),
	}
}
func ValidateAssistantArguments(name string, data []byte) bool {
	var schema map[string]any
	for _, tool := range AssistantToolsV1() {
		if tool.Name == name {
			schema = tool.Schema
			break
		}
	}
	if schema == nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	// Decode members individually to reject duplicate and escaped duplicate keys.
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	values := map[string]any{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		k, ok := key.(string)
		if !ok {
			return false
		}
		if _, exists := values[k]; exists {
			return false
		}
		var value any
		if decoder.Decode(&value) != nil {
			return false
		}
		values[k] = value
	}
	if last, err := decoder.Token(); err != nil || last != json.Delim('}') {
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		return false
	}
	props := schema["properties"].(map[string]any)
	for _, key := range schema["required"].([]string) {
		if _, ok := values[key]; !ok {
			return false
		}
	}
	for key, value := range values {
		raw, exists := props[key]
		if !exists {
			return false
		}
		rule := raw.(map[string]any)
		switch rule["type"] {
		case "string":
			if _, ok := value.(string); !ok {
				return false
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "integer":
			n, ok := value.(json.Number)
			if !ok {
				return false
			}
			i, err := n.Int64()
			if err != nil {
				return false
			}
			if min, ok := rule["minimum"].(int); ok && i < int64(min) {
				return false
			}
			if max, ok := rule["maximum"].(int); ok && i > int64(max) {
				return false
			}
		}
	}
	return true
}
