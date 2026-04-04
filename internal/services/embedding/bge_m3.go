package embedding

import (
	"context"
	"fmt"
	"log/slog"
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
			slog.Info("resource_prepare_done",
				"resource", "bge_m3",
				"file", item.filename,
				"path", item.dst,
				"cached", true,
			)
			continue
		}
		slog.Info("resource_prepare_start",
			"resource", "bge_m3",
			"file", item.filename,
			"path", item.dst,
		)
		if err := os.MkdirAll(filepath.Dir(item.dst), os.ModePerm); err != nil {
			slog.Warn("resource_prepare_failed",
				"resource", "bge_m3",
				"file", item.filename,
				"path", item.dst,
				"stage", "mkdir",
				"err", err,
			)
			return err
		}
		if err := downloadFile(ctx, baseURL+"/"+item.filename, item.dst); err != nil {
			slog.Warn("resource_prepare_failed",
				"resource", "bge_m3",
				"file", item.filename,
				"path", item.dst,
				"stage", "download",
				"err", err,
			)
			return err
		}
		slog.Info("resource_prepare_done",
			"resource", "bge_m3",
			"file", item.filename,
			"path", item.dst,
			"cached", false,
		)
	}

	return nil
}

func resolveTokenizerTargetPath(tokenizerPath string) string {
	if strings.EqualFold(filepath.Ext(tokenizerPath), ".json") {
		return tokenizerPath
	}
	return filepath.Join(tokenizerPath, "tokenizer.json")
}
