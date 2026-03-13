package controllers

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *HTTPController) ListSkills(c *gin.Context) {
	items, err := h.repo.ListSkills()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPController) UploadSkills(c *gin.Context) {
	if h.skillPackage == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "skills 上传服务未初始化"})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请使用 multipart/form-data 上传 zip 文件"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["files[]"]
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "至少上传一个 zip 文件（字段名 files 或 files[]）"})
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
	for _, fh := range files {
		f, openErr := fh.Open()
		if openErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "打开上传文件失败"})
			continue
		}

		data, readErr := io.ReadAll(f)
		_ = f.Close()
		if readErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: "读取上传文件失败"})
			continue
		}

		item, installErr := h.skillPackage.InstallFromZip(fh.Filename, data)
		if installErr != nil {
			resp.Failed = append(resp.Failed, failedItem{File: fh.Filename, Error: installErr.Error()})
			continue
		}
		resp.Success = append(resp.Success, item)
	}

	if len(resp.Success) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "全部上传失败",
			"failed": resp.Failed,
		})
		return
	}

	status := http.StatusOK
	if len(resp.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	c.JSON(status, resp)
}

func (h *HTTPController) DeleteSkill(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不能为空"})
		return
	}
	if h.skillRuntime == nil {
		if err := h.repo.DeleteSkill(id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
		return
	}
	if err := h.skillRuntime.DeleteSkillByID(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
