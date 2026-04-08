package chat

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	llmsvc "slimebot/internal/services/llm"
)

const (
	attachmentCategoryImage    = "image"
	attachmentCategoryAudio    = "audio"
	attachmentCategoryDocument = "document"
	maxInlineAttachmentBytes   = 256 * 1024
)

// classifyAttachmentCategory 将任意上传文件收敛到三类：
// image / audio / document。未知类型统一回收到 document，避免上传被拒绝。
func classifyAttachmentCategory(mimeType, ext string) string {
	mimeLower := strings.ToLower(strings.TrimSpace(mimeType))
	extLower := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	switch {
	case strings.HasPrefix(mimeLower, "image/"):
		return attachmentCategoryImage
	case strings.HasPrefix(mimeLower, "audio/") || extLower == "mp3" || extLower == "wav":
		return attachmentCategoryAudio
	default:
		return attachmentCategoryDocument
	}
}

// detectStoredFileMime 采用“文件头 sniff > 请求头 > 扩展名 > 默认值”优先级。
// 这样可以在前端传错 Content-Type 时做补偿，提升分类稳定性。
func detectStoredFileMime(path, headerMime, ext string) string {
	sniffed := ""
	if file, err := os.Open(path); err == nil {
		defer file.Close()
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		if n > 0 {
			sniffed = strings.ToLower(strings.TrimSpace(http.DetectContentType(buf[:n])))
		}
	}
	if sniffed != "" && sniffed != "application/octet-stream" {
		return sniffed
	}
	normalizedHeader := strings.ToLower(strings.TrimSpace(headerMime))
	if normalizedHeader != "" {
		return normalizedHeader
	}
	byExt := strings.ToLower(strings.TrimSpace(mime.TypeByExtension("." + strings.TrimPrefix(ext, "."))))
	if byExt != "" {
		return byExt
	}
	return "application/octet-stream"
}

// buildUserMessageContentParts 为当前用户回合构建多模态 content parts。
// 返回 fallbackMeta 用于补偿场景：当某个附件构建失败时，调用方可降级到文本提示，
// 保证“至少有附件元信息进入模型”。
func buildUserMessageContentParts(userText string, attachments []UploadedAttachment) ([]llmsvc.ChatMessageContentPart, []string) {
	parts := make([]llmsvc.ChatMessageContentPart, 0, len(attachments)+1)
	if strings.TrimSpace(userText) != "" {
		parts = append(parts, llmsvc.ChatMessageContentPart{
			Type: llmsvc.ChatMessageContentPartTypeText,
			Text: userText,
		})
	}
	fallbackMeta := make([]string, 0)
	for _, file := range attachments {
		part, err := buildAttachmentContentPart(file)
		if err != nil {
			fallbackMeta = append(fallbackMeta, fmt.Sprintf("%s (%s, %d bytes)", file.Name, file.MimeType, file.SizeBytes))
			continue
		}
		parts = append(parts, part)
	}
	return parts, fallbackMeta
}

// buildAttachmentContentPart 将单个附件转换为模型可消费的 part。
// 规则：
// 1) image -> ImageContentPart(data URL)
// 2) audio(wav/mp3) -> InputAudioContentPart
// 3) 其它全部 -> FileContentPart（文档兜底）
func buildAttachmentContentPart(file UploadedAttachment) (llmsvc.ChatMessageContentPart, error) {
	if strings.TrimSpace(file.Path) == "" {
		return llmsvc.ChatMessageContentPart{}, fmt.Errorf("empty file path")
	}
	if file.SizeBytes > maxInlineAttachmentBytes {
		return llmsvc.ChatMessageContentPart{}, fmt.Errorf("attachment too large for inline content")
	}
	raw, err := os.ReadFile(file.Path)
	if err != nil {
		return llmsvc.ChatMessageContentPart{}, err
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	category := strings.TrimSpace(file.Category)
	if category == "" {
		category = classifyAttachmentCategory(file.MimeType, file.Ext)
	}

	switch category {
	case attachmentCategoryImage:
		mimeType := normalizeMimeTypeForDataURL(file.MimeType, file.Ext)
		return llmsvc.ChatMessageContentPart{
			Type:     llmsvc.ChatMessageContentPartTypeImage,
			ImageURL: fmt.Sprintf("data:%s;base64,%s", mimeType, encoded),
		}, nil
	case attachmentCategoryAudio:
		if format, ok := resolveInputAudioFormat(file.MimeType, file.Ext); ok {
			return llmsvc.ChatMessageContentPart{
				Type:             llmsvc.ChatMessageContentPartTypeAudio,
				InputAudioData:   encoded,
				InputAudioFormat: format,
			}, nil
		}
		// 补偿逻辑：SDK 的 input_audio 目前仅支持 wav/mp3，其它音频自动降级为 document。
		fallthrough
	default:
		filename := strings.TrimSpace(file.Name)
		if filename == "" {
			filename = "attachment"
		}
		return llmsvc.ChatMessageContentPart{
			Type:           llmsvc.ChatMessageContentPartTypeFile,
			FileDataBase64: encoded,
			Filename:       filepath.Base(filename),
		}, nil
	}
}

// normalizeMimeTypeForDataURL 生成图片 data URL 所需 mime 类型。
// 当上传元信息不可靠时，用扩展名做次级补偿。
func normalizeMimeTypeForDataURL(mimeType, ext string) string {
	mimeLower := strings.ToLower(strings.TrimSpace(mimeType))
	if mimeLower != "" && mimeLower != "application/octet-stream" {
		return mimeLower
	}
	byExt := strings.ToLower(strings.TrimSpace(mime.TypeByExtension("." + strings.TrimPrefix(ext, "."))))
	if byExt != "" {
		return byExt
	}
	return "application/octet-stream"
}

// resolveInputAudioFormat 将音频 mime/ext 映射为 SDK 支持的输入格式。
// 未命中返回 false，交由上层补偿为 FileContentPart。
func resolveInputAudioFormat(mimeType, ext string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	switch {
	case strings.Contains(m, "wav"), e == "wav":
		return "wav", true
	case strings.Contains(m, "mpeg"), strings.Contains(m, "mp3"), e == "mp3":
		return "mp3", true
	default:
		return "", false
	}
}
