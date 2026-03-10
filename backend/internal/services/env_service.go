package services

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// EnvInfo 描述运行服务的设备环境信息
type EnvInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Shell    string `json:"shell"`
}

// CollectEnvInfo 采集当前设备的环境信息
func CollectEnvInfo() *EnvInfo {
	info := &EnvInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}

	info.Version = detectOSVersion()
	info.Shell = detectShell()

	return info
}

// FormatForPrompt 将环境信息格式化为可嵌入系统提示词的文本
func (e *EnvInfo) FormatForPrompt() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- 操作系统: %s\n", e.OS))
	b.WriteString(fmt.Sprintf("- 系统架构: %s\n", e.Arch))
	if e.Version != "" {
		b.WriteString(fmt.Sprintf("- 系统版本: %s\n", e.Version))
	}
	if e.Hostname != "" {
		b.WriteString(fmt.Sprintf("- 主机名: %s\n", e.Hostname))
	}
	if e.Shell != "" {
		b.WriteString(fmt.Sprintf("- 默认 Shell: %s\n", e.Shell))
	}
	return b.String()
}

func detectOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("cmd", "/C", "ver").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "darwin":
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err == nil {
			return "macOS " + strings.TrimSpace(string(out))
		}
	default:
		// Linux 及其他系统
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					val := strings.TrimPrefix(line, "PRETTY_NAME=")
					val = strings.Trim(val, "\"")
					return val
				}
			}
		}
		out, err := exec.Command("uname", "-r").Output()
		if err == nil {
			return "Linux " + strings.TrimSpace(string(out))
		}
	}
	return ""
}

func detectShell() string {
	switch runtime.GOOS {
	case "windows":
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec
		}
		return "cmd.exe"
	default:
		if shell := os.Getenv("SHELL"); shell != "" {
			return shell
		}
		return "/bin/sh"
	}
}
