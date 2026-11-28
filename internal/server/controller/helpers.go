package controller

import (
	"net/http"
	"strings"

	"slimebot/internal/server/apierrors"
)

func jsonError(c WebContext, status int, message string) {
	apierrors.WriteJSONError(c.Writer(), status, apierrors.APIError{
		Code:    apierrors.CodeForStatus(status),
		Message: message,
	})
}

func jsonInternalError(c WebContext, err error) {
	status, body := apierrors.MapError(err)
	apierrors.WriteJSONError(c.Writer(), status, body)
}

func bindJSONOrBadRequest(c WebContext, req any, message string) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		jsonError(c, http.StatusBadRequest, message)
		return false
	}
	return true
}

func trimSpaceFields(fields ...*string) {
	for _, field := range fields {
		*field = strings.TrimSpace(*field)
	}
}

func lowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func allFieldsPresent(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}
