package embedding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultBgeM3DownloadBaseURL = "https://huggingface.co/BAAI/bge-m3/resolve/main/onnx"

type BgeM3ModelConfig struct {
	ModelPath       string
	TokenizerPath   string
	DownloadBaseURL string
}

func EnsureBgeM3ModelFiles(ctx context.Context, cfg BgeM3ModelConfig) error {
	modelPath := absIfRel(strings.TrimSpace(cfg.ModelPath))
	tokenizerPath := absIfRel(strings.TrimSpace(cfg.TokenizerPath))
	if modelPath == "" || tokenizerPath == "" {
		return fmt.Errorf("bge-m3 requires model_path and tokenizer_path")
	}

	tokenizerJSONPath := resolveTokenizerTargetPath(tokenizerPath)
	modelDataPath := filepath.Join(filepath.Dir(modelPath), "model.onnx_data")
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.DownloadBaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBgeM3DownloadBaseURL
	}

	files := []struct {
		dst      string
		filename string
	}{
		{dst: modelPath, filename: "model.onnx"},
		{dst: modelDataPath, filename: "model.onnx_data"},
		{dst: tokenizerJSONPath, filename: "tokenizer.json"},
	}

	for _, item := range files {
		if isFile(item.dst) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(item.dst), os.ModePerm); err != nil {
			return err
		}
		if err := downloadFile(ctx, baseURL+"/"+item.filename, item.dst); err != nil {
			return err
		}
	}

	return nil
}

func resolveTokenizerTargetPath(tokenizerPath string) string {
	if strings.EqualFold(filepath.Ext(tokenizerPath), ".json") {
		return tokenizerPath
	}
	return filepath.Join(tokenizerPath, "tokenizer.json")
}
