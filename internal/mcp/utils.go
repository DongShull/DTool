package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func textResult(s string) *mcp.CallToolResult {
	return mcp.NewToolResultText(s)
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal error: %v", err))
	}
	return mcp.NewToolResultText(string(b))
}

func mustString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required param: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("param %s must be string", key)
	}
	if s == "" {
		return "", fmt.Errorf("param %s cannot be empty", key)
	}
	return s, nil
}

func optionalBool(args map[string]any, key string, def bool) bool {
	v, ok := args[key]
	if !ok {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}

func optionalString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// optionalStringMap 把 MCP 参数里的字符串映射（JSON object）转成 map[string]string。
// 客户端可能传 map 或 JSON 字符串两种形态，都兼容；非字符串值会报错而不是静默丢弃。
func optionalStringMap(raw any) (map[string]string, error) {
	switch v := raw.(type) {
	case map[string]string:
		return v, nil
	case map[string]any:
		out := make(map[string]string, len(v))
		for k, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("键 %q 的值必须是字符串", k)
			}
			out[k] = s
		}
		return out, nil
	case string:
		var out map[string]string
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, fmt.Errorf("nameMap 不是合法的字符串映射: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("nameMap 必须是对象或 JSON 字符串")
	}
}
