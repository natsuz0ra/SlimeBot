package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type EmbeddingService interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

type ONNXRuntimeEmbeddingConfig struct {
	ModelPath     string
	TokenizerPath string
	PythonBin     string
	ScriptPath    string
	Timeout       time.Duration
	Runner        func(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ONNXRuntimeEmbeddingService struct {
	modelPath     string
	tokenizerPath string
	pythonBin     string
	scriptPath    string
	timeout       time.Duration
	runner        func(ctx context.Context, name string, args ...string) ([]byte, error)
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
	runner := cfg.Runner
	if runner == nil {
		runner = runCommandOutput
	}

	return &ONNXRuntimeEmbeddingService{
		modelPath:     strings.TrimSpace(cfg.ModelPath),
		tokenizerPath: strings.TrimSpace(cfg.TokenizerPath),
		pythonBin:     pythonBin,
		scriptPath:    strings.TrimSpace(cfg.ScriptPath),
		timeout:       timeout,
		runner:        runner,
	}
}

func (s *ONNXRuntimeEmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := s.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding response is empty")
	}
	return vectors[0], nil
}

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

	var response struct {
		Vectors [][]float32 `json:"vectors"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse embedding output failed: %w", err)
	}
	if len(response.Vectors) != len(normalized) {
		return nil, fmt.Errorf("embedding vector count mismatch: want=%d got=%d", len(normalized), len(response.Vectors))
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
