package workspace

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// ValidateScopes accepts repository-relative files or directories. Shell tools
// remain isolated to the whole child workspace; integration enforces this list.
func ValidateScopes(scopes []string) error {
	for _, scope := range scopes {
		if scope == "" || strings.ContainsAny(scope, "\\:*?[]\x00") || path.IsAbs(scope) {
			return fmt.Errorf("invalid write scope: %q", scope)
		}
		for _, part := range strings.Split(scope, "/") {
			if part == ".." {
				return fmt.Errorf("invalid write scope: %q", scope)
			}
		}
	}
	return nil
}

func checkScope(raw, file string) error {
	if raw == "" {
		return nil
	}
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return err
	}
	if err := ValidateScopes(scopes); err != nil {
		return err
	}
	if len(scopes) == 0 {
		return nil
	}
	file = path.Clean(file)
	for _, scope := range scopes {
		scope = path.Clean(scope)
		if scope == "." || file == scope || strings.HasPrefix(file, scope+"/") {
			return nil
		}
	}
	return fmt.Errorf("file outside assigned write scopes: %s", file)
}
