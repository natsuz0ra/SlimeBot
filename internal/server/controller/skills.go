package controller

import (
	"io"
	"net/http"
	"strings"
)

// ListSkills returns installed skill packages.
func (h *HTTPController) ListSkills(c WebContext) {
	if h.skillRuntime == nil {
		jsonError(c, http.StatusInternalServerError, "Skills runtime service is not initialized.")
		return
	}
	items, err := h.skillRuntime.ListSkills()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// UploadSkills installs multiple skill zips and reports per-file success/failure.
func (h *HTTPController) UploadSkills(c WebContext) {
	if h.skillPackage == nil {
		jsonError(c, http.StatusInternalServerError, "Skills upload service is not initialized.")
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Please upload ZIP files using multipart/form-data.")
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["files[]"]
	}
	if len(files) == 0 {
		jsonError(c, http.StatusBadRequest, "At least one ZIP file is required (field name: files or files[]).")
		return
	}

	type failedItem struct {
		File  string `json:"file"`
		Error string `json:"error"`
	}
	type uploadResp struct {
		Success []any        `json:"success"`
		Failed  []failedItem `json:"failed"`
	}

	resp := uploadResp{
		Success: make([]any, 0, len(files)),
		Failed:  make([]failedItem, 0),
	}
	// Install one file at a time so one failure does not abort the batch.
	for _, fh := range files {
		f, openErr := fh.Open()
		if openErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "Failed to open uploaded file."})
			continue
		}

		data, readErr := io.ReadAll(f)
		if closeErr := f.Close(); closeErr != nil {
			c.Error(closeErr)
		}
		if readErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "Failed to read uploaded file."})
			continue
		}

		item, installErr := h.skillPackage.InstallFromZip(fh.Filename, data)
		if installErr != nil {
			c.Error(installErr)
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "Installation failed."})
			continue
		}
		resp.Success = append(resp.Success, item)
	}
	if len(resp.Success) > 0 && h.skillRuntime != nil {
		h.skillRuntime.InvalidateCatalog()
	}

	if len(resp.Success) == 0 {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":  "All uploads failed.",
			"failed": resp.Failed,
		})
		return
	}

	// 207 Multi-Status when some uploads failed so the UI can surface partial errors.
	status := http.StatusOK
	if len(resp.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	c.JSON(status, resp)
}

// DeleteSkill removes a skill; prefers runtime delete to keep in-memory state in sync.
func (h *HTTPController) DeleteSkill(c WebContext) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		jsonError(c, http.StatusBadRequest, "id is required.")
		return
	}
	if h.skillRuntime == nil {
		jsonError(c, http.StatusInternalServerError, "Skills runtime service is not initialized.")
		return
	}
	if err := h.skillRuntime.DeleteSkillByID(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SetSkillEnabled updates the SlimeBot enabled flag for an external skill.
func (h *HTTPController) SetSkillEnabled(c WebContext) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		jsonError(c, http.StatusBadRequest, "id is required.")
		return
	}
	if h.skillRuntime == nil {
		jsonError(c, http.StatusInternalServerError, "Skills runtime service is not initialized.")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if err := h.skillRuntime.SetSkillEnabled(id, req.Enabled); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
