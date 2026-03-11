package tools

// CommandParam 描述工具命令的一个参数
type CommandParam struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

// Command 描述工具支持的一条命令
type Command struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Params      []CommandParam `json:"params,omitempty"`
}

// ExecuteResult 是工具命令的执行结果
type ExecuteResult struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Tool 是所有工具必须实现的接口。
// 新增工具只需在 tools 包下新建文件，实现此接口并在 init() 中调用 Register 即可自动注册。
type Tool interface {
	// Name 返回工具的唯一标识名称，如 "exec"、"http_request"
	Name() string
	// Description 返回工具的简短功能描述
	Description() string
	// Commands 返回工具支持的所有命令列表
	Commands() []Command
	// Execute 执行指定命令，params 的 key 为参数名，value 为参数值
	Execute(command string, params map[string]string) (*ExecuteResult, error)
}
