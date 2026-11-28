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

// classifyAttachmentCategory maps uploads to image / audio / document.
// Unknown types become document so uploads are not rejected.
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

// detectStoredFileMime prefers magic-byte sniff, then header mime, extension, then octet-stream.
// Helps when the client sends a wrong Content-Type.
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

// buildUserMessageContentParts builds multimodal parts for the current user turn.
// fallbackMeta lists attachments that failed to inline so callers can fall back to text metadata.
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

// buildAttachmentContentPart converts one upload to a model part.
// Rules: image -> data URL; wav/mp3 -> input_audio; everything else -> file part.
func buildAttachmentContentPart(file UploadedAttachment) (llmsvc.ChatMessageContentPart, error) {
	if strings.TrimSpace(file.Path) == "" {
		return llmsvc.ChatMessageContentPart{}, fmt.Errorf("empty file path")
	}
	if file.SizeBytes > maxInlineAttachmentBytes && !file.inlineImage {
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
		// SDK input_audio supports wav/mp3 only; other audio becomes a generic file part.
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

// normalizeMimeTypeForDataURL picks a mime for image data URLs; falls back to extension.
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

// resolveInputAudioFormat maps mime/extension to SDK input_audio format.
// Returns false to let the caller fall back to FileContentPart.
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
