package privacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"slimebot/internal/services/privacy/inference"
)

const modelRevision = "7216b6a3b897ef903d4b1ba367ae6fb95b0fa5e4"
const modelBase = "https://huggingface.co/QuantumNous/astrlink-guard/resolve/" + modelRevision + "/"

type modelFile struct {
	remote, name, sha string
	size              int64
}

var modelFiles = []modelFile{
	{"int8/model_int8.onnx", "model_int8.onnx", "0a070d3113615e33ba3b84d6bd486f663a377c04c4b056e7381732179f51832a", 330781},
	{"int8/model_int8.onnx_data", "model_int8.onnx_data", "0ba34216ceead01e0ae2dfd8e46d52b67e7aee80b04f722d9b758613a91c0067", 37124608},
	{"int8/tokenizer.json", "tokenizer.json", "0cf3b235ef125658015a6c8026a5fe6f28088bf825101a990ec18a10200feef4", 1502881},
	{"config.json", "config.json", "2c20003590d642e273c03c4c7c82d9849fa75f64bf29a71ec0a0c9a20ba4a150", 1804},
	{"LICENSE", "LICENSE", "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", 11357},
}

type ModelStatus struct {
	Installed       bool   `json:"installed"`
	Installing      bool   `json:"installing"`
	Running         bool   `json:"running"`
	Stage           string `json:"stage"`
	DownloadedBytes int64  `json:"downloadedBytes"`
	TotalBytes      int64  `json:"totalBytes"`
	Error           string `json:"error,omitempty"`
	Runtime         string `json:"runtime"`
}

type LocalModel struct {
	dir        string
	mu         sync.Mutex
	gate       chan struct{}
	status     ModelStatus
	cancel     context.CancelFunc
	model      *inference.Model
	scanCancel context.CancelFunc
	epoch      uint64
	idle       *time.Timer
}

func NewLocalModel(dir string) *LocalModel {
	return &LocalModel{dir: dir, gate: make(chan struct{}, 1), status: ModelStatus{Stage: "not_installed", Runtime: "Go / AstrLink Guard INT8 (CPU)"}}
}

func (m *LocalModel) installed() bool {
	for _, f := range modelFiles {
		info, err := os.Stat(filepath.Join(m.dir, f.name))
		if err != nil || info.Size() != f.size {
			return false
		}
	}
	return true
}
func (m *LocalModel) Status() ModelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.status
	v.Installed = m.installed()
	v.Running = m.model != nil
	for _, f := range modelFiles {
		v.TotalBytes += f.size
	}
	if !v.Installing && v.Installed {
		v.Stage = "ready"
	}
	return v
}

func (m *LocalModel) Install() ModelStatus {
	select {
	case m.gate <- struct{}{}:
	default:
		v := m.Status()
		if !v.Installing {
			v.Error = "local model is busy; try installing after the request ends"
		}
		return v
	}

	m.mu.Lock()
	if m.status.Installing {
		<-m.gate
		m.mu.Unlock()
		return m.Status()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	m.cancel = cancel
	m.status.Installing = true
	m.status.Error = ""
	m.status.Stage = "preparing"
	m.status.DownloadedBytes = 0
	m.mu.Unlock()
	go func() {
		defer func() { <-m.gate }()
		err := m.install(ctx)
		cancel()
		m.mu.Lock()
		m.status.Installing = false
		m.cancel = nil
		if err != nil {
			if errors.Is(err, context.Canceled) {
				m.status.Error = "installation cancelled"
			} else {
				m.status.Error = err.Error()
			}
			m.status.Stage = "error"
		} else {
			m.status.Stage = "ready"
		}
		m.mu.Unlock()
	}()
	return m.Status()
}
func (m *LocalModel) Cancel() {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *LocalModel) install(ctx context.Context) error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return errors.New("cannot create model directory")
	}
	for _, file := range modelFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		m.status.Stage = "downloading"
		m.mu.Unlock()
		if validModelFile(filepath.Join(m.dir, file.name), file) {
			m.mu.Lock()
			m.status.DownloadedBytes += file.size
			m.mu.Unlock()
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelBase+file.remote, nil)
		if err != nil {
			return err
		}
		resp, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
		if err != nil {
			return errors.New("model download failed")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return errors.New("model download returned an error")
		}
		path := filepath.Join(m.dir, file.name+".part")
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			resp.Body.Close()
			return errors.New("cannot write model file")
		}
		h := sha256.New()
		reader := io.LimitReader(resp.Body, file.size+1)
		buffer := make([]byte, 128<<10)
		var count int64
		for {
			n, readErr := reader.Read(buffer)
			if n > 0 {
				_, err = io.Copy(io.MultiWriter(dst, h), bytes.NewReader(buffer[:n]))
				if err != nil {
					break
				}
				count += int64(n)
				m.mu.Lock()
				m.status.DownloadedBytes += int64(n)
				m.mu.Unlock()
			}
			if readErr != nil {
				if readErr != io.EOF {
					err = readErr
				}
				break
			}
		}
		closeErr := dst.Close()
		resp.Body.Close()
		if err != nil || closeErr != nil || count != file.size || hex.EncodeToString(h.Sum(nil)) != file.sha {
			os.Remove(path)
			return errors.New("model download integrity check failed")
		}
		target := filepath.Join(m.dir, file.name)
		if err = os.Remove(target); err != nil && !os.IsNotExist(err) {
			return errors.New("cannot replace model file")
		}
		if err = os.Rename(path, target); err != nil {
			return errors.New("cannot install verified model file")
		}
	}
	return ctx.Err()
}

func validModelFile(path string, f modelFile) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(file, f.size+1))
	return err == nil && n == f.size && hex.EncodeToString(h.Sum(nil)) == f.sha
}

func (m *LocalModel) start(ctx context.Context) (*inference.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.model != nil {
		return m.model, nil
	}
	if m.status.Installing || !m.installed() {
		return nil, errors.New("install the local privacy model first")
	}
	for _, f := range modelFiles {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !validModelFile(filepath.Join(m.dir, f.name), f) {
			return nil, errors.New("local model integrity check failed; reinstall the model")
		}
	}
	model, err := inference.Load(ctx, m.dir)
	if err != nil {
		return nil, err
	}
	m.model = model
	return model, nil
}

func (m *LocalModel) Detect(ctx context.Context, text string, threshold float64) ([]span, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case m.gate <- struct{}{}:
		defer func() { <-m.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.mu.Lock()
	m.epoch++
	epoch := m.epoch
	m.scanCancel = cancel
	if m.idle != nil {
		m.idle.Stop()
		m.idle = nil
	}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.scanCancel = nil
		if m.model != nil {
			m.idle = time.AfterFunc(3*time.Minute, func() {
				select {
				case m.gate <- struct{}{}:
					defer func() { <-m.gate }()
				default:
					return
				}
				m.mu.Lock()
				defer m.mu.Unlock()
				if m.epoch == epoch {
					m.model = nil
					m.idle = nil
				}
			})
		}
		m.mu.Unlock()
	}()
	model, err := m.start(ctx)
	if err != nil {
		return nil, err
	}
	findings, err := model.Detect(ctx, text, threshold)
	if err != nil {
		return nil, err
	}
	out := make([]span, 0, len(findings))
	for _, v := range findings {
		out = append(out, span{v.Start, v.End, v.Kind, "model"})
	}
	return out, nil
}

func (m *LocalModel) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scanCancel != nil {
		m.scanCancel()
	}
	m.model = nil
	if m.idle != nil {
		m.idle.Stop()
		m.idle = nil
	}
}

func (m *LocalModel) Remove() error {
	select {
	case m.gate <- struct{}{}:
		defer func() { <-m.gate }()
	default:
		return errors.New("local model is scanning; try removing it after the request ends")
	}
	m.mu.Lock()
	busy := m.status.Installing
	m.mu.Unlock()
	if busy {
		return errors.New("cancel model installation before removing it")
	}
	m.Stop()
	for _, f := range modelFiles {
		if err := os.Remove(filepath.Join(m.dir, f.name)); err != nil && !os.IsNotExist(err) {
			return errors.New("cannot remove model file")
		}
		_ = os.Remove(filepath.Join(m.dir, f.name+".part"))
	}
	for _, name := range []string{"runtime.json", "detector.py"} {
		_ = os.Remove(filepath.Join(m.dir, name))
	}
	return nil
}
