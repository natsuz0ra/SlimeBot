package platforms

// InboundMessage 是平台侧入站消息的统一结构。
type InboundMessage struct {
	Platform string
	ChatID   string
	Text     string
}

// OutboundSender 抽象平台消息发送能力，便于在 dispatcher 中复用同一处理流程。
type OutboundSender interface {
	SendText(chatID string, text string) error
	SendApprovalKeyboard(chatID string, text string, approveData string, rejectData string) error
}
