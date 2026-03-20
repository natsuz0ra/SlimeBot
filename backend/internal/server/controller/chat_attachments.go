package controller

import (
	"net/http"
	"strings"
)

type uploadSessionAttachmentsResponse struct {
	Items []attachmentUploadItem `json:"items"`
}

type attachmentUploadItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ext       string `json:"ext"`
	SizeBytes int64  `json:"sizeBytes"`
	MimeType  string `json:"mimeType"`
	Category  string `json:"category,omitempty"`
	IconType  string `json:"iconType"`
}

// UploadSessionAttachments 上传聊天附件，返回临时附件标识供本次会话使用。
// 该接口只负责“暂存并返回引用”，不直接参与模型推理。
func (h *HTTPController) UploadSessionAttachments(c WebContext) {
	if h.chatUploads == nil {
		jsonError(c, http.StatusInternalServerError, "Chat upload service is not initialized.")
		return
	}
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		jsonError(c, http.StatusBadRequest, "session id is required.")
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Please upload files using multipart/form-data.")
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["files[]"]
	}
	// 兼容 files / files[] 两种前端字段命名。
	if len(files) == 0 {
		jsonError(c, http.StatusBadRequest, "At least one file is required (field name: files or files[]).")
		return
	}
	items, saveErr := h.chatUploads.SaveFiles(sessionID, files)
	if saveErr != nil {
		jsonError(c, http.StatusBadRequest, saveErr.Error())
		return
	}
	resp := uploadSessionAttachmentsResponse{
		Items: make([]attachmentUploadItem, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, attachmentUploadItem{
			ID:        item.ID,
			Name:      item.Name,
			Ext:       item.Ext,
			SizeBytes: item.SizeBytes,
			MimeType:  item.MimeType,
			Category:  item.Category,
			IconType:  item.IconType,
		})
	}
	// 返回的是“附件元信息 + 临时 ID”，后续由 chat 请求通过 attachmentIds 消费。
	c.JSON(http.StatusOK, resp)
}
