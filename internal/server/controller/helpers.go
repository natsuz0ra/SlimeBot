package controller

import (
	"net/http"
	apierrors2 "slimebot/internal/server/apierrors"
	"strings"
)

func jsonError(c WebContext, status int, message string) {
	apierrors2.WriteJSONError(c.Writer(), status, apierrors2.APIError{Message: message})
}

func jsonInternalError(c WebContext, err error) {
	if err != nil {
		c.Error(err)
	}
	jsonError(c, http.StatusInternalServerError, "internal server error")
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
		if field == nil {
			continue
		}
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
