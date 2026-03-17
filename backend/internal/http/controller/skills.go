package controller

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ListSkills 返回已安装技能包列表。
func (h *HTTPController) ListSkills(c *gin.Context) {
	items, err := h.repo.ListSkills()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// UploadSkills 批量上传并安装技能 zip，返回成功与失败明细。
func (h *HTTPController) UploadSkills(c *gin.Context) {
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
	// 按文件逐个安装，确保单个失败不会影响整批处理。
	for _, fh := range files {
		f, openErr := fh.Open()
		if openErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "Failed to open uploaded file."})
			continue
		}

		data, readErr := io.ReadAll(f)
		if closeErr := f.Close(); closeErr != nil {
			_ = c.Error(closeErr)
		}
		if readErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "Failed to read uploaded file."})
			continue
		}

		item, installErr := h.skillPackage.InstallFromZip(fh.Filename, data)
		if installErr != nil {
			_ = c.Error(installErr)
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "Installation failed."})
			continue
		}
		resp.Success = append(resp.Success, item)
	}

	if len(resp.Success) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "All uploads failed.",
			"failed": resp.Failed,
		})
		return
	}

	// 部分成功使用 207，便于前端提示“有失败项”。
	status := http.StatusOK
	if len(resp.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	c.JSON(status, resp)
}

// DeleteSkill 删除指定技能；优先走 runtime 以保持运行态一致。
func (h *HTTPController) DeleteSkill(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		jsonError(c, http.StatusBadRequest, "id is required.")
		return
	}
	if h.skillRuntime == nil {
		if err := h.repo.DeleteSkill(id); err != nil {
			jsonInternalError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
		return
	}
	if err := h.skillRuntime.DeleteSkillByID(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
