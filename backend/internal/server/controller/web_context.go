package controller

import (
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// WebContext 将控制器从 gin 解耦到通用请求上下文抽象。
// 仅覆盖当前控制器使用到的最小集合方法。
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

// chiContext 适配 chi/net/http 的 *http.Request 与 ResponseWriter。
type chiContext struct {
	w http.ResponseWriter
	r *http.Request
}

// NewChiContext 构造一个可用于 controller.WebContext 的适配对象。
func NewChiContext(w http.ResponseWriter, r *http.Request) WebContext {
	return chiContext{w: w, r: r}
}

func (c chiContext) Writer() http.ResponseWriter { return c.w }
func (c chiContext) Request() *http.Request      { return c.r }

func (c chiContext) Param(key string) string {
	return chi.URLParam(c.r, key)
}

func (c chiContext) Query(key string) string {
	return c.r.URL.Query().Get(key)
}

func (c chiContext) GetHeader(key string) string { return c.r.Header.Get(key) }

func (c chiContext) Status(code int) {
	c.w.WriteHeader(code)
}

func (c chiContext) JSON(code int, v any) {
	// 保持与 apierrors.WriteJSONError 一致的 json 编码行为；失败仅记录，避免二次写出崩溃。
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(code)
	if err := json.NewEncoder(c.w).Encode(v); err != nil {
		log.Printf("json_encode_failed err=%v", err)
	}
}

func (c chiContext) Error(err error) {
	if err == nil {
		return
	}
	// 当前控制器主要使用 c.Error(err) 作为“记录”，不会影响响应体。
	log.Printf("handler_error err=%v", err)
}

func (c chiContext) ShouldBindJSON(dst any) error {
	if dst == nil {
		return io.ErrUnexpectedEOF
	}
	dec := json.NewDecoder(io.LimitReader(c.r.Body, 1<<20))
	err := dec.Decode(dst)
	// 对空 body 保持 gin 行为：返回 io.EOF 以便调用方做特定兼容（例如允许 name 为空）。
	if err == io.EOF {
		return io.EOF
	}
	return err
}

func (c chiContext) MultipartForm() (*multipart.Form, error) {
	// 参照 gin 默认的 ParseMultipartForm 行为；若后续想调大/调小可改这里。
	const maxMemory = 32 << 20 // 32MB
	if err := c.r.ParseMultipartForm(maxMemory); err != nil {
		return nil, err
	}
	return c.r.MultipartForm, nil
}
