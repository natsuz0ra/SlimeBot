package platforms

import (
	"encoding/json"
	"fmt"
	"strings"

	"slimebot/internal/constants"
)

type telegramAuthConfig struct {
	BotToken string `json:"botToken"`
}

// ValidateAuthConfig 校验平台鉴权 JSON，避免保存后在运行期失败。
func ValidateAuthConfig(platform string, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("auth config is empty")
	}
	var asObject map[string]any
	if err := json.Unmarshal([]byte(trimmed), &asObject); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(platform), constants.TelegramPlatformName) {
		if strings.TrimSpace(ParseTelegramBotToken(trimmed)) == "" {
			return fmt.Errorf("telegram botToken is required")
		}
	}
	return nil
}

// ParseTelegramBotToken 从 Telegram 平台配置中提取 botToken，失败时返回空串。
func ParseTelegramBotToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var cfg telegramAuthConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.BotToken)
}
