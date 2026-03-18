package telegram

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/platforms"
	"slimebot/backend/internal/repositories"
)

type Worker struct {
	repo            *repositories.Repository
	dispatcher      *platforms.Dispatcher
	dispatchInbound func(context.Context, platforms.InboundMessage, platforms.OutboundSender) error
	dispatchSlots   chan struct{}
}

const (
	workerMaxConcurrentDispatch = 8
	workerDispatchTimeout       = 180 * time.Second
)

func NewWorker(repo *repositories.Repository, dispatcher *platforms.Dispatcher) *Worker {
	w := &Worker{
		repo:          repo,
		dispatcher:    dispatcher,
		dispatchSlots: make(chan struct{}, workerMaxConcurrentDispatch),
	}
	w.dispatchInbound = func(ctx context.Context, inbound platforms.InboundMessage, sender platforms.OutboundSender) error {
		return dispatcher.HandleInbound(ctx, inbound, sender)
	}
	return w
}

// Start 启动后台轮询循环；出现错误时会按固定退避间隔重试。
func (w *Worker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	go w.run(ctx)
}

// run 持续执行 Telegram 长轮询：
// - 拉取平台配置并检查启用状态；
// - 基于 updateOffset 增量消费更新，避免重复处理；
// - 将文本消息转给 dispatcher，将按钮回调转给审批处理。
func (w *Worker) run(ctx context.Context) {
	var updateOffset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg, err := w.repo.GetMessagePlatformConfigByPlatform(consts.TelegramPlatformName)
		if err != nil {
			log.Printf("telegram_worker_load_config_failed err=%v", err)
			time.Sleep(consts.TelegramErrorBackoff)
			continue
		}
		if cfg == nil || !cfg.IsEnabled {
			time.Sleep(consts.TelegramIdleWaitInterval)
			continue
		}

		token := platforms.ParseTelegramBotToken(cfg.AuthConfigJSON)
		if token == "" {
			time.Sleep(consts.TelegramIdleWaitInterval)
			continue
		}
		adapter := NewAdapter(token)
		updates, err := adapter.GetUpdates(ctx, updateOffset, consts.TelegramPollTimeoutSeconds)
		if err != nil {
			log.Printf("telegram_worker_poll_failed err=%v", err)
			time.Sleep(consts.TelegramErrorBackoff)
			continue
		}

		updateOffset = w.processUpdates(ctx, adapter, updates, updateOffset)
	}
}

// processUpdates 逐条处理 Telegram 更新并推进 offset。
func (w *Worker) processUpdates(ctx context.Context, adapter *Adapter, updates []update, updateOffset int64) int64 {
	nextOffset := updateOffset
	for _, item := range updates {
		if item.UpdateID >= nextOffset {
			nextOffset = item.UpdateID + 1
		}
		if item.CallbackQuery != nil {
			w.handleApprovalCallback(item.CallbackQuery, adapter)
			continue
		}
		if item.Message == nil {
			continue
		}
		w.dispatchInboundAsync(ctx, platforms.InboundMessage{
			Platform: consts.TelegramPlatformName,
			ChatID:   strconv.FormatInt(item.Message.Chat.ID, 10),
			Text:     strings.TrimSpace(item.Message.Text),
		}, adapter)
	}
	return nextOffset
}

// dispatchInboundAsync 异步执行文本消息分发，避免审批等待阻塞轮询主循环。
func (w *Worker) dispatchInboundAsync(ctx context.Context, inbound platforms.InboundMessage, sender platforms.OutboundSender) {
	if w == nil || sender == nil {
		return
	}
	chatID := strings.TrimSpace(inbound.ChatID)
	if strings.TrimSpace(inbound.Text) == "" {
		return
	}
	select {
	case w.dispatchSlots <- struct{}{}:
	default:
		log.Printf("telegram_worker_dispatch_throttled chat_id=%s", chatID)
		_ = sender.SendText(chatID, "System is busy. Please try again later.")
		return
	}

	go func() {
		defer func() { <-w.dispatchSlots }()
		taskCtx, cancel := context.WithTimeout(ctx, workerDispatchTimeout)
		defer cancel()
		if err := w.dispatchInbound(taskCtx, inbound, sender); err != nil {
			log.Printf("telegram_worker_dispatch_failed chat_id=%s err=%v", chatID, err)
			_ = sender.SendText(chatID, "Failed to process the message. Please try again later.")
		}
	}()
}

// handleApprovalCallback 处理审批按钮点击，并即时回执给 Telegram 客户端。
func (w *Worker) handleApprovalCallback(query *callbackQuery, adapter *Adapter) {
	if w == nil || query == nil || adapter == nil {
		return
	}
	if query.Message == nil {
		log.Printf("telegram_worker_callback_missing_message callback_id=%s", strings.TrimSpace(query.ID))
		_ = adapter.AnswerCallbackQuery(query.ID, "Approval message context is missing.")
		return
	}
	chatID := strconv.FormatInt(query.Message.Chat.ID, 10)
	data := strings.TrimSpace(query.Data)
	log.Printf("telegram_worker_callback_received chat_id=%s callback_id=%s", chatID, strings.TrimSpace(query.ID))
	if data == "" {
		log.Printf("telegram_worker_callback_empty_data chat_id=%s callback_id=%s", chatID, strings.TrimSpace(query.ID))
		_ = adapter.AnswerCallbackQuery(query.ID, "Approval command is empty.")
		return
	}
	approved, err := w.dispatcher.HandleTelegramApprovalCallback(chatID, data)
	if err != nil {
		log.Printf("telegram_worker_callback_resolve_failed chat_id=%s callback_id=%s err=%v", chatID, strings.TrimSpace(query.ID), err)
		_ = adapter.AnswerCallbackQuery(query.ID, "Approval failed: "+err.Error())
		return
	}
	if approved {
		log.Printf("telegram_worker_callback_approved chat_id=%s callback_id=%s", chatID, strings.TrimSpace(query.ID))
		_ = adapter.AnswerCallbackQuery(query.ID, "Approved. Executing now.")
		return
	}
	log.Printf("telegram_worker_callback_rejected chat_id=%s callback_id=%s", chatID, strings.TrimSpace(query.ID))
	_ = adapter.AnswerCallbackQuery(query.ID, "Execution has been rejected.")
}
