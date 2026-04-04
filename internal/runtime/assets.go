package runtime

import _ "embed"

//go:embed env.template
var envTemplate string

func EnvTemplate() string {
	return envTemplate
}
