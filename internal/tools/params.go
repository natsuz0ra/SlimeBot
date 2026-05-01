package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func paramString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func paramStringTrim(params map[string]any, key string) string {
	return strings.TrimSpace(paramString(params, key))
}

func paramBool(params map[string]any, key string) (bool, bool, error) {
	if params == nil {
		return false, false, nil
	}
	v, ok := params[key]
	if !ok || v == nil {
		return false, false, nil
	}
	switch t := v.(type) {
	case bool:
		return t, true, nil
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "" {
			return false, false, nil
		}
		b, err := strconv.ParseBool(s)
		if err != nil {
			return false, false, fmt.Errorf("%s must be true or false", key)
		}
		return b, true, nil
	default:
		return false, false, fmt.Errorf("%s must be a boolean", key)
	}
}

func paramInt(params map[string]any, key string) (int, bool, error) {
	if params == nil {
		return 0, false, nil
	}
	v, ok := params[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	switch t := v.(type) {
	case float64:
		return int(t), true, nil
	case int:
		return t, true, nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false, nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return 0, false, fmt.Errorf("%s must be an integer", key)
		}
		return i, true, nil
	default:
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
}

func decodeParamInto(params map[string]any, key string, target any) (bool, error) {
	if params == nil {
		return false, nil
	}
	v, ok := params[key]
	if !ok || v == nil {
		return false, nil
	}
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return false, nil
		}
		if err := json.Unmarshal([]byte(t), target); err != nil {
			return true, err
		}
		return true, nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return true, err
		}
		if err := json.Unmarshal(b, target); err != nil {
			return true, err
		}
		return true, nil
	}
}
