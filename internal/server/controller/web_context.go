package controller

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"slimebot/internal/logging"

	"github.com/go-chi/chi/v5"
)

// WebContext decouples controllers from Gin with a minimal request abstraction.
type WebContext interface {
	Writer() http.ResponseWriter
	Request() *http.Request

	Param(key string) string
	Query(key string) string
	GetHeader(key string) string

	Status(code int)
	JSON(code int, v any)
	Error(err error)

	ShouldBindJSON(dst any) error
	MultipartForm() (*multipart.Form, error)
}

// chiContext adapts chi's *http.Request and ResponseWriter.
type chiContext struct {
	w http.ResponseWriter
	r *http.Request
}

// NewChiContext builds a WebContext for chi handlers.
func NewChiContext(w http.ResponseWriter, r *http.Request) WebContext {
	return chiContext{w: w, r: r}
}

func (c chiContext) Writer() http.ResponseWriter { return c.w }
func (c chiContext) Request() *http.Request      { return c.r }

func (c chiContext) Param(key string) string {
	value := chi.URLParam(c.r, key)
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func (c chiContext) Query(key string) string {
	return c.r.URL.Query().Get(key)
}

func (c chiContext) GetHeader(key string) string { return c.r.Header.Get(key) }

func (c chiContext) Status(code int) {
	c.w.WriteHeader(code)
}

func (c chiContext) JSON(code int, v any) {
	// Match apierrors JSON encoding; log failures to avoid double-write panics.
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(code)
	if err := json.NewEncoder(c.w).Encode(v); err != nil {
		logging.Warn("json_encode_failed", "err", err)
	}
}

func (c chiContext) Error(err error) {
	if err == nil {
		return
	}
	// Controllers mostly use Error for logging; it does not change the response body.
	logging.Warn("handler_error", "err", err)
}

func (c chiContext) ShouldBindJSON(dst any) error {
	dec := json.NewDecoder(io.LimitReader(c.r.Body, 1<<20))
	return dec.Decode(dst)
}

func (c chiContext) MultipartForm() (*multipart.Form, error) {
	// Mirrors Gin's default ParseMultipartForm memory limit; tune here if needed.
	const maxMemory = 32 << 20 // 32MB
	if err := c.r.ParseMultipartForm(maxMemory); err != nil {
		return nil, err
	}
	return c.r.MultipartForm, nil
}
