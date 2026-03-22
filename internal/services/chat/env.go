package chat

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// EnvInfo 描述运行服务的设备环境信息
type EnvInfo struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	Hostname       string `json:"hostname"`
	Version        string `json:"version"`
	Shell          string `json:"shell"`
	CurrentDate    string `json:"current_date"`
	CurrentTime    string `json:"current_time"`
	Timezone       string `json:"timezone"`
	TimezoneOffset string `json:"timezone_offset"`
}

var (
	staticEnvInfo     EnvInfo
	staticEnvInfoOnce sync.Once
)

// CollectEnvInfo 采集当前设备的环境信息
func CollectEnvInfo() *EnvInfo {
	staticEnvInfoOnce.Do(func() {
		staticEnvInfo = EnvInfo{
			OS:      runtime.GOOS,
			Arch:    runtime.GOARCH,
			Version: detectOSVersion(),
			Shell:   detectShell(),
		}
		if hostname, err := os.Hostname(); err == nil {
			staticEnvInfo.Hostname = hostname
		}
	})

	now := time.Now()
	info := &EnvInfo{
		OS:             staticEnvInfo.OS,
		Arch:           staticEnvInfo.Arch,
		Hostname:       staticEnvInfo.Hostname,
		Version:        staticEnvInfo.Version,
		Shell:          staticEnvInfo.Shell,
		CurrentDate:    now.Format("2006-01-02"),
		CurrentTime:    now.Format("15:04:05"),
		Timezone:       now.Location().String(),
		TimezoneOffset: now.Format("-07:00"),
	}

	return info
}

// FormatForPrompt 将环境信息格式化为可嵌入系统提示词的文本
func (e *EnvInfo) FormatForPrompt() string {
	var b strings.Builder
	if e.CurrentDate != "" {
		b.WriteString(fmt.Sprintf("- Local date: %s\n", e.CurrentDate))
	}
	if e.CurrentTime != "" {
		b.WriteString(fmt.Sprintf("- Local time: %s\n", e.CurrentTime))
	}
	if e.Timezone != "" {
		if e.TimezoneOffset != "" {
			b.WriteString(fmt.Sprintf("- Timezone: %s (UTC%s)\n", e.Timezone, e.TimezoneOffset))
		} else {
			b.WriteString(fmt.Sprintf("- Timezone: %s\n", e.Timezone))
		}
	}
	b.WriteString(fmt.Sprintf("- OS: %s\n", e.OS))
	b.WriteString(fmt.Sprintf("- Architecture: %s\n", e.Arch))
	if e.Version != "" {
		b.WriteString(fmt.Sprintf("- OS version: %s\n", e.Version))
	}
	if e.Hostname != "" {
		b.WriteString(fmt.Sprintf("- Hostname: %s\n", e.Hostname))
	}
	if e.Shell != "" {
		b.WriteString(fmt.Sprintf("- Default shell: %s\n", e.Shell))
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
