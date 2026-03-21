package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Adapter struct {
	token string
	http  *http.Client
}

type getUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []update `json:"result"`
}

type update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *message       `json:"message"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}

type message struct {
	MessageID int64            `json:"message_id"`
	Chat      chat             `json:"chat"`
	Text      string           `json:"text"`
	Caption   string           `json:"caption"`
	Photo     []photoSize      `json:"photo"`
	Voice     *voiceAttachment `json:"voice"`
	Audio     *audioAttachment `json:"audio"`
	Document  *docAttachment   `json:"document"`
}

type photoSize struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
}

type voiceAttachment struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type audioAttachment struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type docAttachment struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type chat struct {
	ID int64 `json:"id"`
}

type callbackQuery struct {
	ID      string   `json:"id"`
	From    user     `json:"from"`
	Message *message `json:"message"`
	Data    string   `json:"data"`
}

type user struct {
	ID int64 `json:"id"`
}

type getFileResponse struct {
	OK     bool          `json:"ok"`
	Result *telegramFile `json:"result"`
}

type telegramFile struct {
	FilePath string `json:"file_path"`
}

type mediaCandidate struct {
	Source         string
	ProviderFileID string
	Name           string
	MimeType       string
	SizeBytes      int64
}

func NewAdapter(token string) *Adapter {
	return &Adapter{
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 45 * time.Second},
	}
}

func (a *Adapter) getAPIURL(method string) string {
	return "https://api.telegram.org/bot" + a.token + "/" + method
}

func (a *Adapter) getFileURL(filePath string) string {
	return "https://api.telegram.org/file/bot" + a.token + "/" + strings.TrimLeft(strings.TrimSpace(filePath), "/")
}

// GetUpdates 调用 Telegram getUpdates 长轮询接口拉取增量更新。
// 该方法同时校验 HTTP 状态码与 payload.ok，避免将异常响应当作有效数据。
func (a *Adapter) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]update, error) {
	if a == nil || strings.TrimSpace(a.token) == "" {
		return nil, fmt.Errorf("telegram token is empty")
	}

	query := url.Values{}
	query.Set("timeout", strconv.Itoa(timeoutSeconds))
	if offset > 0 {
		query.Set("offset", strconv.FormatInt(offset, 10))
	}
	apiURL := a.getAPIURL("getUpdates") + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram getUpdates failed: status=%d", resp.StatusCode)
	}
	var payload getUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram getUpdates returned ok=false")
	}
	return payload.Result, nil
}

func (a *Adapter) ResolveFilePath(ctx context.Context, fileID string) (string, error) {
	if a == nil || strings.TrimSpace(a.token) == "" {
		return "", fmt.Errorf("telegram token is empty")
	}
	trimmedFileID := strings.TrimSpace(fileID)
	if trimmedFileID == "" {
		return "", fmt.Errorf("telegram file id is empty")
	}
	query := url.Values{}
	query.Set("file_id", trimmedFileID)
	apiURL := a.getAPIURL("getFile") + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("telegram getFile failed: status=%d", resp.StatusCode)
	}
	var payload getFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if !payload.OK || payload.Result == nil || strings.TrimSpace(payload.Result.FilePath) == "" {
		return "", fmt.Errorf("telegram getFile returned empty file path")
	}
	return payload.Result.FilePath, nil
}

func (a *Adapter) DownloadFile(ctx context.Context, fileID string, maxBytes int64) ([]byte, string, error) {
	filePath, err := a.ResolveFilePath(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	data, err := a.DownloadFileByPath(ctx, filePath, maxBytes)
	if err != nil {
		return nil, "", err
	}
	return data, filepath.Base(strings.TrimSpace(filePath)), nil
}

func (a *Adapter) DownloadFileByPath(ctx context.Context, filePath string, maxBytes int64) ([]byte, error) {
	if a == nil || strings.TrimSpace(a.token) == "" {
		return nil, fmt.Errorf("telegram token is empty")
	}
	trimmedPath := strings.TrimSpace(filePath)
	if trimmedPath == "" {
		return nil, fmt.Errorf("telegram file path is empty")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("max bytes must be greater than 0")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.getFileURL(trimmedPath), nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram file download failed: status=%d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("telegram file exceeds max size limit")
	}
	return body, nil
}

func collectMediaCandidates(msg *message) []mediaCandidate {
	if msg == nil {
		return nil
	}
	items := make([]mediaCandidate, 0, 4)
	if len(msg.Photo) > 0 {
		best := msg.Photo[0]
		for _, p := range msg.Photo[1:] {
			if p.FileSize > best.FileSize {
				best = p
			}
		}
		if strings.TrimSpace(best.FileID) != "" {
			items = append(items, mediaCandidate{
				Source:         "photo",
				ProviderFileID: strings.TrimSpace(best.FileID),
				Name:           "photo.jpg",
				MimeType:       "image/jpeg",
				SizeBytes:      best.FileSize,
			})
		}
	}
	if msg.Voice != nil && strings.TrimSpace(msg.Voice.FileID) != "" {
		items = append(items, mediaCandidate{
			Source:         "voice",
			ProviderFileID: strings.TrimSpace(msg.Voice.FileID),
			Name:           "voice.ogg",
			MimeType:       strings.TrimSpace(msg.Voice.MimeType),
			SizeBytes:      msg.Voice.FileSize,
		})
	}
	if msg.Audio != nil && strings.TrimSpace(msg.Audio.FileID) != "" {
		items = append(items, mediaCandidate{
			Source:         "audio",
			ProviderFileID: strings.TrimSpace(msg.Audio.FileID),
			Name:           strings.TrimSpace(msg.Audio.FileName),
			MimeType:       strings.TrimSpace(msg.Audio.MimeType),
			SizeBytes:      msg.Audio.FileSize,
		})
	}
	if msg.Document != nil && strings.TrimSpace(msg.Document.FileID) != "" {
		items = append(items, mediaCandidate{
			Source:         "document",
			ProviderFileID: strings.TrimSpace(msg.Document.FileID),
			Name:           strings.TrimSpace(msg.Document.FileName),
			MimeType:       strings.TrimSpace(msg.Document.MimeType),
			SizeBytes:      msg.Document.FileSize,
		})
	}
	return items
}

// SendText 发送纯文本消息，作为平台统一回包能力。
func (a *Adapter) SendText(chatID string, text string) error {
	if a == nil || strings.TrimSpace(a.token) == "" {
		return fmt.Errorf("telegram token is empty")
	}
	payload := map[string]any{
		"chat_id": strings.TrimSpace(chatID),
		"text":    text,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, a.getAPIURL("sendMessage"), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage failed: status=%d", resp.StatusCode)
	}
	return nil
}

// SendApprovalKeyboard 发送带 Inline Keyboard 的审批消息；
// callback_data 会被 approval broker 解析为批准/拒绝指令。
func (a *Adapter) SendApprovalKeyboard(chatID string, text string, approveData string, rejectData string) error {
	if a == nil || strings.TrimSpace(a.token) == "" {
		return fmt.Errorf("telegram token is empty")
	}
	payload := map[string]any{
		"chat_id": strings.TrimSpace(chatID),
		"text":    text,
		"reply_markup": map[string]any{
			"inline_keyboard": []any{
				[]map[string]string{
					{"text": "Approve", "callback_data": approveData},
					{"text": "Reject", "callback_data": rejectData},
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, a.getAPIURL("sendMessage"), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage(approval) failed: status=%d", resp.StatusCode)
	}
	return nil
}

// AnswerCallbackQuery 应答按钮点击，避免 Telegram 客户端持续转圈。
func (a *Adapter) AnswerCallbackQuery(callbackQueryID string, text string) error {
	if a == nil || strings.TrimSpace(a.token) == "" {
		return fmt.Errorf("telegram token is empty")
	}
	payload := map[string]any{
		"callback_query_id": strings.TrimSpace(callbackQueryID),
	}
	if strings.TrimSpace(text) != "" {
		payload["text"] = text
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, a.getAPIURL("answerCallbackQuery"), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram answerCallbackQuery failed: status=%d", resp.StatusCode)
	}
	return nil
}
