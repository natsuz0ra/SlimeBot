package controller

import (
	"context"
	"errors"
	"net/http"

	acpclient "slimebot/internal/acp/client"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"
)

func (h *HTTPController) SetACPServices(configs *configsvc.ACPConfigService, client *acpclient.Manager, reset func(context.Context, string) error) {
	h.acpConfigs, h.acpClient, h.resetACP = configs, client, reset
}

func (h *HTTPController) ListACPConfigs(c WebContext) {
	items, err := h.acpConfigs.List(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPController) CreateACPConfig(c WebContext) {
	var req configsvc.ACPConfigInput
	if !bindJSONOrBadRequest(c, &req, "Invalid ACP configuration.") {
		return
	}
	item, err := h.acpConfigs.Create(c.Request().Context(), req)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *HTTPController) UpdateACPConfig(c WebContext) {
	var req configsvc.ACPConfigInput
	if !bindJSONOrBadRequest(c, &req, "Invalid ACP configuration.") {
		return
	}
	item, err := h.acpConfigs.Update(c.Request().Context(), c.Param("id"), req)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	h.acpClient.ForgetConfig(item.ID)
	c.JSON(http.StatusOK, item)
}

func (h *HTTPController) DeleteACPConfig(c WebContext) {
	id := c.Param("id")
	if err := h.acpConfigs.Delete(c.Request().Context(), id); err != nil {
		jsonInternalError(c, err)
		return
	}
	h.acpClient.ForgetConfig(id)
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) TestACPConnection(c WebContext) {
	result, err := h.acpClient.TestConnection(c.Request().Context(), c.Param("id"))
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *HTTPController) ResetACPSession(c WebContext) {
	err := h.resetACP(c.Request().Context(), c.Param("id"))
	if errors.Is(err, chatsvc.ErrTurnBusy) {
		jsonError(c, http.StatusConflict, "A turn is running for this session.")
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
