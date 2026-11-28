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

// UploadSessionAttachments stages chat files and returns temporary attachment IDs for the session.
// This endpoint only stores refs; model inference consumes them via chat requests.
func (h *HTTPController) UploadSessionAttachments(c WebContext) {
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
	// Accept both "files" and "files[]" form field names.
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
	// Response is metadata plus temp IDs; chat requests consume them via attachmentIds.
	c.JSON(http.StatusOK, resp)
}
