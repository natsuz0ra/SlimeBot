package platforms

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"slimebot/backend/internal/repositories"
)

const (
	// 轮询参数固定在代码常量中，避免运行时配置漂移影响行为。
	telegramPollTimeoutSeconds = 25
	telegramIdleWaitInterval   = 60 * time.Second
	telegramErrorBackoff       = 5 * time.Second
	telegramPlatformName       = "telegram"
)

type telegramWorkerAuthConfig struct {
	BotToken string `json:"botToken"`
}

type TelegramWorker struct {
	repo       *repositories.Repository
	dispatcher *Dispatcher
}

func NewTelegramWorker(repo *repositories.Repository, dispatcher *Dispatcher) *TelegramWorker {
	return &TelegramWorker{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

// Start 启动后台轮询循环；出现错误时会按固定退避间隔重试。
func (w *TelegramWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	go w.run(ctx)
}

func (w *TelegramWorker) run(ctx context.Context) {
	var updateOffset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg, err := w.repo.GetMessagePlatformConfigByPlatform(telegramPlatformName)
		if err != nil {
			log.Printf("telegram_worker_load_config_failed err=%v", err)
			time.Sleep(telegramErrorBackoff)
			continue
		}
		if cfg == nil || !cfg.IsEnabled {
			time.Sleep(telegramIdleWaitInterval)
			continue
		}

		token := parseTelegramToken(cfg.AuthConfigJSON)
		if token == "" {
			time.Sleep(telegramIdleWaitInterval)
			continue
		}
		adapter := NewTelegramAdapter(token)
		updates, err := adapter.GetUpdates(ctx, updateOffset, telegramPollTimeoutSeconds)
		if err != nil {
			log.Printf("telegram_worker_poll_failed err=%v", err)
			time.Sleep(telegramErrorBackoff)
			continue
		}

		for _, item := range updates {
			if item.UpdateID >= updateOffset {
				updateOffset = item.UpdateID + 1
			}
			if item.Message == nil {
				continue
			}
			text := strings.TrimSpace(item.Message.Text)
			if text == "" {
				continue
			}
			chatID := strconv.FormatInt(item.Message.Chat.ID, 10)
			inbound := InboundMessage{
				Platform: telegramPlatformName,
				ChatID:   chatID,
				Text:     text,
			}
			if err := w.dispatcher.HandleInbound(ctx, inbound, adapter); err != nil {
				log.Printf("telegram_worker_dispatch_failed chat_id=%s err=%v", chatID, err)
				_ = adapter.SendText(chatID, "处理消息失败，请稍后重试。")
			}
		}
	}
}

func parseTelegramToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var cfg telegramWorkerAuthConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.BotToken)
}
