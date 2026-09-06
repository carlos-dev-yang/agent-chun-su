package mail

import "encoding/json"

const ExecutorSchemaName = "executor.schema.json"

// ExecutorSchema keeps the host contract intact while projecting the observed
// unsupported Structured Outputs keyword out of the generation-only schema.
func ExecutorSchema(data []byte) ([]byte, error) {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	var visit func(any)
	visit = func(value any) {
		node, ok := value.(map[string]any)
		if !ok {
			return
		}
		delete(node, "uniqueItems")
		for _, key := range []string{"properties", "$defs", "definitions", "patternProperties", "dependentSchemas"} {
			if children, ok := node[key].(map[string]any); ok {
				for _, child := range children {
					visit(child)
				}
			}
		}
		for _, key := range []string{"items", "additionalProperties", "unevaluatedProperties", "contains", "propertyNames", "not", "if", "then", "else"} {
			visit(node[key])
		}
		for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
			if children, ok := node[key].([]any); ok {
				for _, child := range children {
					visit(child)
				}
			}
		}
	}
	visit(document)
	return json.MarshalIndent(document, "", "  ")
}
