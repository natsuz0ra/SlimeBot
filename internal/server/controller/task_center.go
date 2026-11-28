package controller

import (
	"context"
	"net/http"
	"slimebot/internal/domain"
)

type taskCenterService interface {
	TaskCenter(context.Context) (domain.TaskCenter, error)
}

func (h *HTTPController) SetTaskCenterService(service taskCenterService) { h.taskCenter = service }

func (h *HTTPController) GetTaskCenter(c WebContext) {
	result, err := h.taskCenter.TaskCenter(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
