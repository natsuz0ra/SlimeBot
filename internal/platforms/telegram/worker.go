package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/platforms"
	"slimebot/internal/repositories"
	chatsvc "slimebot/internal/services/chat"
)

type Worker struct {
	repo            *repositories.Repository
	dispatcher      *platforms.Dispatcher
	uploads         workerUploadService
	dispatchInbound func(context.Context, platforms.InboundMessage, platforms.OutboundSender) error
	dispatchSlots   chan struct{}
}

type workerUploadService interface {
	RegisterLocalFiles(sessionID string, files []chatsvc.LocalAttachmentFile) ([]chatsvc.UploadedAttachment, error)
}

const (
	workerMaxConcurrentDispatch = 8
	workerDispatchTimeout       = 180 * time.Second
	workerMaxMediaBytes         = 10 * 1024 * 1024
	workerMaxAttachmentsPerMsg  = 5
)

func NewWorker(repo *repositories.Repository, dispatcher *platforms.Dispatcher, uploads workerUploadService) *Worker {
	w := &Worker{
		repo:          repo,
		dispatcher:    dispatcher,
		uploads:       uploads,
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
	var adapter *Adapter
	lastToken := ""
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg, err := w.repo.GetMessagePlatformConfigByPlatform(constants.TelegramPlatformName)
		if err != nil {
			slog.Warn("telegram_worker_load_config_failed", "err", err)
			time.Sleep(constants.TelegramErrorBackoff)
			continue
		}
		if cfg == nil || !cfg.IsEnabled {
			time.Sleep(constants.TelegramIdleWaitInterval)
			continue
		}

		token := platforms.ParseTelegramBotToken(cfg.AuthConfigJSON)
		if token == "" {
			adapter = nil
			lastToken = ""
			time.Sleep(constants.TelegramIdleWaitInterval)
			continue
		}
		if adapter == nil || token != lastToken {
			adapter = NewAdapter(token)
			lastToken = token
		}
		updates, err := adapter.GetUpdates(ctx, updateOffset, constants.TelegramPollTimeoutSeconds)
		if err != nil {
			slog.Warn("telegram_worker_poll_failed", "err", err)
			time.Sleep(constants.TelegramErrorBackoff)
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
		chatID := strconv.FormatInt(item.Message.Chat.ID, 10)
		text := strings.TrimSpace(item.Message.Text)
		if text == "" {
			text = strings.TrimSpace(item.Message.Caption)
		}
		inbound := platforms.InboundMessage{
			Platform: constants.TelegramPlatformName,
			ChatID:   chatID,
			Text:     text,
		}
		candidates := collectMediaCandidates(item.Message)
		if len(candidates) > 0 {
			attachmentIDs, attachments, warnErr := w.buildInboundAttachments(ctx, adapter, candidates)
			inbound.AttachmentIDs = attachmentIDs
			inbound.Attachments = attachments
			if warnErr != nil {
				slog.Warn("telegram_worker_media_partial_failed", "chat_id", chatID, "err", warnErr)
				if err := adapter.SendText(chatID, "Some attachments failed to process and were skipped."); err != nil {
					slog.Warn("telegram_send_text_failed", "chat_id", chatID, "err", err)
				}
			}
		}
		if strings.TrimSpace(inbound.Text) == "" && len(inbound.AttachmentIDs) == 0 {
			continue
		}
		w.dispatchInboundAsync(ctx, platforms.InboundMessage{
			Platform:      inbound.Platform,
			ChatID:        inbound.ChatID,
			Text:          inbound.Text,
			Attachments:   inbound.Attachments,
			AttachmentIDs: inbound.AttachmentIDs,
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
	if strings.TrimSpace(inbound.Text) == "" && len(inbound.AttachmentIDs) == 0 {
		return
	}
	select {
	case w.dispatchSlots <- struct{}{}:
	default:
		slog.Warn("telegram_worker_dispatch_throttled", "chat_id", chatID)
		if err := sender.SendText(chatID, "System is busy. Please try again later."); err != nil {
			slog.Warn("telegram_send_text_failed", "chat_id", chatID, "err", err)
		}
		return
	}

	go func() {
		defer func() { <-w.dispatchSlots }()
		taskCtx, cancel := context.WithTimeout(ctx, workerDispatchTimeout)
		defer cancel()
		if err := w.dispatchInbound(taskCtx, inbound, sender); err != nil {
			slog.Warn("telegram_worker_dispatch_failed", "chat_id", chatID, "err", err)
			if sendErr := sender.SendText(chatID, "Failed to process the message. Please try again later."); sendErr != nil {
				slog.Warn("telegram_send_text_failed", "chat_id", chatID, "err", sendErr)
			}
		}
	}()
}

func (w *Worker) buildInboundAttachments(ctx context.Context, adapter *Adapter, candidates []mediaCandidate) ([]string, []platforms.InboundAttachment, error) {
	if w == nil || w.uploads == nil || adapter == nil || len(candidates) == 0 {
		return nil, nil, nil
	}
	if len(candidates) > workerMaxAttachmentsPerMsg {
		candidates = candidates[:workerMaxAttachmentsPerMsg]
	}
	inputs := make([]chatsvc.LocalAttachmentFile, 0, len(candidates))
	inboundMeta := make([]platforms.InboundAttachment, 0, len(candidates))
	var skipped int

	type downloadedCandidate struct {
		idx          int
		candidate    mediaCandidate
		data         []byte
		fallbackName string
		err          error
	}
	downloads := make([]downloadedCandidate, len(candidates))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, candidate := range candidates {
		wg.Add(1)
		go func(i int, candidate mediaCandidate) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				downloads[i] = downloadedCandidate{idx: i, candidate: candidate, err: ctx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			data, fallbackName, err := adapter.DownloadFile(ctx, candidate.ProviderFileID, workerMaxMediaBytes)
			downloads[i] = downloadedCandidate{
				idx:          i,
				candidate:    candidate,
				data:         data,
				fallbackName: fallbackName,
				err:          err,
			}
		}(i, candidate)
	}
	wg.Wait()

	for _, downloaded := range downloads {
		if downloaded.err != nil {
			skipped++
			continue
		}
		name := selectAttachmentName(downloaded.candidate.Name, downloaded.fallbackName, downloaded.candidate.Source, downloaded.candidate.MimeType)
		inputs = append(inputs, chatsvc.LocalAttachmentFile{
			Name:     name,
			MimeType: downloaded.candidate.MimeType,
			Data:     downloaded.data,
		})
		inboundMeta = append(inboundMeta, platforms.InboundAttachment{
			Source:         downloaded.candidate.Source,
			ProviderFileID: downloaded.candidate.ProviderFileID,
			Name:           name,
			MimeType:       strings.TrimSpace(downloaded.candidate.MimeType),
			SizeBytes:      int64(len(downloaded.data)),
		})
	}
	if len(inputs) == 0 {
		if skipped > 0 {
			return nil, nil, fmt.Errorf("no attachment can be downloaded")
		}
		return nil, nil, nil
	}

	registered, err := w.uploads.RegisterLocalFiles(constants.MessagePlatformSessionID, inputs)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, 0, len(registered))
	for i, item := range registered {
		ids = append(ids, item.ID)
		if i < len(inboundMeta) {
			inboundMeta[i].Category = strings.TrimSpace(item.Category)
			inboundMeta[i].MimeType = strings.TrimSpace(item.MimeType)
			inboundMeta[i].SizeBytes = item.SizeBytes
			if strings.TrimSpace(item.Name) != "" {
				inboundMeta[i].Name = item.Name
			}
		}
	}
	if skipped > 0 {
		return ids, inboundMeta, fmt.Errorf("%d attachments failed to process", skipped)
	}
	return ids, inboundMeta, nil
}

func selectAttachmentName(preferred string, fallback string, source string, mimeType string) string {
	name := strings.TrimSpace(preferred)
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" {
		name = strings.TrimSpace(source)
	}
	if name == "" {
		name = "attachment"
	}
	ext := strings.TrimSpace(filepath.Ext(name))
	if ext != "" {
		return name
	}
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return name + ".jpg"
	case "image/png":
		return name + ".png"
	case "audio/mpeg":
		return name + ".mp3"
	case "audio/wav", "audio/x-wav":
		return name + ".wav"
	case "audio/ogg":
		return name + ".ogg"
	case "application/pdf":
		return name + ".pdf"
	default:
		return name
	}
}

// handleApprovalCallback 处理审批按钮点击，并即时回执给 Telegram 客户端。
func (w *Worker) handleApprovalCallback(query *callbackQuery, adapter *Adapter) {
	if w == nil || query == nil || adapter == nil {
		return
	}
	if query.Message == nil {
		slog.Warn("telegram_worker_callback_missing_message", "callback_id", strings.TrimSpace(query.ID))
		if err := adapter.AnswerCallbackQuery(query.ID, "Approval message context is missing."); err != nil {
			slog.Warn("telegram_answer_callback_failed", "callback_id", query.ID, "err", err)
		}
		return
	}
	chatID := strconv.FormatInt(query.Message.Chat.ID, 10)
	data := strings.TrimSpace(query.Data)
	slog.Info("telegram_worker_callback_received", "chat_id", chatID, "callback_id", strings.TrimSpace(query.ID))
	if data == "" {
		slog.Warn("telegram_worker_callback_empty_data", "chat_id", chatID, "callback_id", strings.TrimSpace(query.ID))
		if err := adapter.AnswerCallbackQuery(query.ID, "Approval command is empty."); err != nil {
			slog.Warn("telegram_answer_callback_failed", "callback_id", query.ID, "err", err)
		}
		return
	}
	approved, err := w.dispatcher.HandleTelegramApprovalCallback(chatID, data)
	if err != nil {
		slog.Warn("telegram_worker_callback_resolve_failed", "chat_id", chatID, "callback_id", strings.TrimSpace(query.ID), "err", err)
		if aerr := adapter.AnswerCallbackQuery(query.ID, "Approval failed: "+err.Error()); aerr != nil {
			slog.Warn("telegram_answer_callback_failed", "callback_id", query.ID, "err", aerr)
		}
		return
	}
	if approved {
		slog.Info("telegram_worker_callback_approved", "chat_id", chatID, "callback_id", strings.TrimSpace(query.ID))
		if err := adapter.AnswerCallbackQuery(query.ID, "Approved. Executing now."); err != nil {
			slog.Warn("telegram_answer_callback_failed", "callback_id", query.ID, "err", err)
		}
		return
	}
	slog.Info("telegram_worker_callback_rejected", "chat_id", chatID, "callback_id", strings.TrimSpace(query.ID))
	if err := adapter.AnswerCallbackQuery(query.ID, "Execution has been rejected."); err != nil {
		slog.Warn("telegram_answer_callback_failed", "callback_id", query.ID, "err", err)
	}
}
