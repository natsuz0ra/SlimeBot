package controller

import (
	"context"
	"errors"
	"net/http"
	"time"

	privacysvc "slimebot/internal/services/privacy"
)

func (h *HTTPController) privacyReady(c WebContext) bool {
	if h.privacy == nil {
		jsonError(c, http.StatusServiceUnavailable, "Privacy service is unavailable.")
		return false
	}
	return true
}

func (h *HTTPController) GetPrivacy(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	snapshot, err := h.privacy.Snapshot(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *HTTPController) UpdatePrivacy(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	input := struct {
		privacysvc.Config
		MappingPersistence *bool `json:"mappingPersistence"`
		MappingTTLMinutes  *int  `json:"mappingTtlMinutes"`
	}{Config: privacysvc.DefaultConfig()}
	if !bindJSONOrBadRequest(c, &input, "Invalid privacy configuration.") {
		return
	}
	if input.MappingTTLMinutes != nil {
		input.Config.MappingTTLMinutes = *input.MappingTTLMinutes
	}
	if err := input.Config.Validate(); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.privacy.UpdateWithMappingSettings(c.Request().Context(), input.Config, input.MappingPersistence, input.MappingTTLMinutes); err != nil {
		if errors.Is(err, privacysvc.ErrMappingConfiguration) {
			jsonError(c, http.StatusBadRequest, err.Error())
		} else {
			jsonInternalError(c, err)
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) PreviewPrivacy(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	var input struct {
		Text   string             `json:"text"`
		Config *privacysvc.Config `json:"config,omitempty"`
	}
	c.Request().Body = http.MaxBytesReader(c.Writer(), c.Request().Body, 2<<20)
	if !bindJSONOrBadRequest(c, &input, "Invalid preview input.") {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Minute)
	defer cancel()
	preview, err := h.privacy.PreviewWithConfig(ctx, input.Text, input.Config)
	if err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (h *HTTPController) ClearPrivacyRecords(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	h.privacy.ClearRecords()
	c.Status(http.StatusNoContent)
}
func (h *HTTPController) InstallPrivacyModel(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	c.JSON(http.StatusAccepted, h.privacy.Model().Install())
}
func (h *HTTPController) CancelPrivacyModel(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	h.privacy.Model().Cancel()
	c.JSON(http.StatusAccepted, h.privacy.Model().Status())
}
func (h *HTTPController) RemovePrivacyModel(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	config, err := h.privacy.GetConfig(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	config.ModelEnabled = false
	if err = h.privacy.Update(c.Request().Context(), config); err != nil {
		jsonInternalError(c, err)
		return
	}
	if err = h.privacy.Model().Remove(); err != nil {
		jsonError(c, http.StatusConflict, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}
