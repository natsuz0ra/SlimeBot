package embedding

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EmbeddingService 文本向量化统一接口（单条与批量）。
type EmbeddingService interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// ONNXRuntimeEmbeddingConfig ONNX 嵌入：模型/分词器路径、Python 与脚本、超时；Runner 非空则走一次性子进程模式。
type ONNXRuntimeEmbeddingConfig struct {
	ModelPath     string
	TokenizerPath string
	PythonBin     string
	ScriptPath    string
	Timeout       time.Duration
	Runner        func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ONNXRuntimeEmbeddingService 基于 Python ONNX 脚本的嵌入：默认长驻管道，或注入 Runner 时每批子进程。
type ONNXRuntimeEmbeddingService struct {
	modelPath     string
	tokenizerPath string
	pythonBin     string
	scriptPath    string
	timeout       time.Duration
	runner        func(ctx context.Context, name string, args ...string) ([]byte, error)
	subprocess    bool

	cache *embeddingLRU

	inflightMu sync.Mutex
	inflight   map[string][]chan embedResult

	pipeMu     sync.Mutex
	pipeCmd    *exec.Cmd
	pipeStdin  *bufio.Writer
	pipeStdout *bufio.Reader
}

type embedResult struct {
	vector []float32
	err    error
}

func absIfRel(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Join(wd, filepath.Clean(p))
}

func NewONNXRuntimeEmbeddingService(cfg ONNXRuntimeEmbeddingConfig) *ONNXRuntimeEmbeddingService {
	pythonBin := strings.TrimSpace(cfg.PythonBin)
	if pythonBin == "" {
		pythonBin = "python"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	subprocess := cfg.Runner != nil
	runner := cfg.Runner
	if subprocess && runner == nil {
		runner = runCommandOutput
	}

	return &ONNXRuntimeEmbeddingService{
		modelPath:     absIfRel(strings.TrimSpace(cfg.ModelPath)),
		tokenizerPath: absIfRel(strings.TrimSpace(cfg.TokenizerPath)),
		pythonBin:     pythonBin,
		scriptPath:    absIfRel(strings.TrimSpace(cfg.ScriptPath)),
		timeout:       timeout,
		runner:        runner,
		subprocess:    subprocess,
		cache:         newEmbeddingLRU(512),
		inflight:      make(map[string][]chan embedResult),
	}
}

// StartPipe 预热管道模式下的 Python 子进程（子进程模式无操作）。
func (s *ONNXRuntimeEmbeddingService) StartPipe(ctx context.Context) error {
	if s.subprocess {
		return nil
	}
	startCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	s.pipeMu.Lock()
	defer s.pipeMu.Unlock()
	select {
	case <-startCtx.Done():
		return startCtx.Err()
	default:
	}
	return s.ensurePipeProcess()
}

// Embed 单条嵌入：LRU 缓存命中即返回；否则对同文本并发去重（singleflight），再调 EmbedBatch。
func (s *ONNXRuntimeEmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}
	if cached, ok := s.getCachedVector(trimmed); ok {
		return cached, nil
	}
	waitCh := s.registerInflight(trimmed)
	if waitCh != nil {
		select {
		case result := <-waitCh:
			return result.vector, result.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	vectors, err := s.EmbedBatch(ctx, []string{trimmed})
	if err != nil {
		s.finishInflight(trimmed, nil, err)
		return nil, err
	}
	if len(vectors) == 0 {
		err = fmt.Errorf("embedding response is empty")
		s.finishInflight(trimmed, nil, err)
		return nil, err
	}
	s.setCachedVector(trimmed, vectors[0])
	s.finishInflight(trimmed, vectors[0], nil)
	return vectors[0], nil
}

// registerInflight 若已有同文本在算，则登记等待通道并返回；否则占位返回 nil 表示当前协程负责计算。
func (s *ONNXRuntimeEmbeddingService) registerInflight(text string) chan embedResult {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if waiters, ok := s.inflight[text]; ok {
		ch := make(chan embedResult, 1)
		s.inflight[text] = append(waiters, ch)
		return ch
	}
	s.inflight[text] = []chan embedResult{}
	return nil
}

// finishInflight 广播本次计算结果给所有等待同文本的协程并清空 inflight 键。
func (s *ONNXRuntimeEmbeddingService) finishInflight(text string, vector []float32, err error) {
	s.inflightMu.Lock()
	waiters := s.inflight[text]
	delete(s.inflight, text)
	s.inflightMu.Unlock()
	for _, ch := range waiters {
		ch <- embedResult{vector: vector, err: err}
		close(ch)
	}
}

func (s *ONNXRuntimeEmbeddingService) getCachedVector(text string) ([]float32, bool) {
	return s.cache.get(text)
}

func (s *ONNXRuntimeEmbeddingService) setCachedVector(text string, vector []float32) {
	s.cache.put(text, vector)
}

// EmbedBatch 批量嵌入：去空后按配置走子进程或 stdin/stdout 管道；空输入返回空切片。
func (s *ONNXRuntimeEmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	normalized := make([]string, 0, len(texts))
	for _, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return [][]float32{}, nil
	}
	if s.modelPath == "" || s.tokenizerPath == "" || s.scriptPath == "" {
		return nil, fmt.Errorf("onnx embedding config requires model_path, tokenizer_path and script_path")
	}

	if s.subprocess {
		return s.embedBatchSubprocess(ctx, normalized)
	}
	return s.embedBatchPipe(ctx, normalized)
}

// embedBatchSubprocess 每次调用启动 Python 子进程，经 --texts-json 传入文本并解析 stdout JSON。
func (s *ONNXRuntimeEmbeddingService) embedBatchSubprocess(ctx context.Context, normalized []string) ([][]float32, error) {
	args := []string{
		s.scriptPath,
		"--model-path", s.modelPath,
		"--tokenizer-path", s.tokenizerPath,
		"--texts-json", mustJSON(normalized),
	}
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	output, err := s.runner(runCtx, s.pythonBin, args...)
	if err != nil {
		return nil, fmt.Errorf("onnx embedding runner failed: %w", err)
	}
	vectors, err := parseEmbeddingOutput(output)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(normalized) {
		return nil, fmt.Errorf("embedding vector count mismatch: want=%d got=%d", len(normalized), len(vectors))
	}
	return vectors, nil
}

// embedBatchPipe 管道模式：最多重试一次，失败则杀进程以便下次 ensure 重建。
func (s *ONNXRuntimeEmbeddingService) embedBatchPipe(ctx context.Context, normalized []string) ([][]float32, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.pipeMu.Lock()
		if err := s.ensurePipeProcess(); err != nil {
			s.pipeMu.Unlock()
			lastErr = err
			continue
		}
		vecs, err := s.pipeRoundTrip(ctx, normalized)
		s.pipeMu.Unlock()
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		s.pipeMu.Lock()
		s.killPipeProcess()
		s.pipeMu.Unlock()
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("onnx embedding pipe failed")
}

// ensurePipeProcess 懒启动或复用已存活 Python 进程，并绑定 stdin/stdout。
func (s *ONNXRuntimeEmbeddingService) ensurePipeProcess() error {
	if s.pipeCmd != nil && s.pipeCmd.Process != nil && s.pipeCmd.ProcessState == nil {
		return nil
	}
	s.killPipeProcess()
	cmd := exec.Command(s.pythonBin, s.scriptPath, "--model-path", s.modelPath, "--tokenizer-path", s.tokenizerPath)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.pipeCmd = cmd
	s.pipeStdin = bufio.NewWriter(stdin)
	s.pipeStdout = bufio.NewReader(stdout)
	return nil
}

func (s *ONNXRuntimeEmbeddingService) killPipeProcess() {
	if s.pipeCmd == nil {
		return
	}
	if s.pipeCmd.Process != nil {
		_ = s.pipeCmd.Process.Kill()
		_ = s.pipeCmd.Wait()
	}
	s.pipeCmd = nil
	s.pipeStdin = nil
	s.pipeStdout = nil
}

// pipeRoundTrip 向 stdin 写入一行 JSON（含 texts），从 stdout 读一行解析 vectors。
func (s *ONNXRuntimeEmbeddingService) pipeRoundTrip(ctx context.Context, texts []string) ([][]float32, error) {
	req := map[string][]string{"texts": texts}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if _, err := s.pipeStdin.Write(payload); err != nil {
		return nil, err
	}
	if err := s.pipeStdin.Flush(); err != nil {
		return nil, err
	}
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := s.pipeStdout.ReadBytes('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- string(bytes.TrimSpace(line))
	}()
	var raw string
	select {
	case <-runCtx.Done():
		s.killPipeProcess()
		return nil, runCtx.Err()
	case err := <-errCh:
		return nil, err
	case raw = <-lineCh:
	}
	vectors, err := parseEmbeddingOutput([]byte(raw))
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("embedding vector count mismatch: want=%d got=%d", len(texts), len(vectors))
	}
	return vectors, nil
}

func parseEmbeddingOutput(output []byte) ([][]float32, error) {
	var response struct {
		Vectors [][]float32 `json:"vectors"`
		Error   string      `json:"error"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse embedding output failed: %w", err)
	}
	if strings.TrimSpace(response.Error) != "" {
		return nil, fmt.Errorf("embedding error: %s", response.Error)
	}
	return response.Vectors, nil
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func runCommandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w stderr=%s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return output, nil
}

// Close 关闭管道子进程（子进程嵌入模式无操作）。
func (s *ONNXRuntimeEmbeddingService) Close() error {
	if s == nil || s.subprocess {
		return nil
	}
	s.pipeMu.Lock()
	defer s.pipeMu.Unlock()
	s.killPipeProcess()
	return nil
}
