package tools

import (
	"fmt"
	"log"
	"sync"
)

var (
	globalRegistry = &Registry{tools: make(map[string]Tool)}
)

// Registry 管理所有已注册的工具
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// Register 将工具注册到全局注册中心。通常在各工具文件的 init() 中调用。
// 如果工具名称重复会 panic，确保每个工具名称唯一。
func Register(tool Tool) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	name := tool.Name()
	if _, exists := globalRegistry.tools[name]; exists {
		panic(fmt.Sprintf("工具名称重复: %s", name))
	}
	globalRegistry.tools[name] = tool
	log.Printf("tool_registered name=%s commands=%d", name, len(tool.Commands()))
}

// Get 根据名称获取已注册的工具
func Get(name string) (Tool, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	t, ok := globalRegistry.tools[name]
	return t, ok
}

// All 返回所有已注册工具的列表
func All() []Tool {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	result := make([]Tool, 0, len(globalRegistry.tools))
	for _, t := range globalRegistry.tools {
		result = append(result, t)
	}
	return result
}

// AllMap 返回工具名称到工具实例的映射
func AllMap() map[string]Tool {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	result := make(map[string]Tool, len(globalRegistry.tools))
	for name, t := range globalRegistry.tools {
		result[name] = t
	}
	return result
}
