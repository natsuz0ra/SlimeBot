package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	MessageID int64  `json:"message_id"`
	Chat      chat   `json:"chat"`
	Text      string `json:"text"`
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

func NewAdapter(token string) *Adapter {
	return &Adapter{
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 45 * time.Second},
	}
}

func (a *Adapter) getAPIURL(method string) string {
	return "https://api.telegram.org/bot" + a.token + "/" + method
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
