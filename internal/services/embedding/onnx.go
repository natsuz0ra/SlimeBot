package embedding

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

// EmbeddingService 文本向量化统一接口（单条与批量）。
type EmbeddingService interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

type ONNXRuntimeGoEmbeddingConfig struct {
	ModelPath        string
	TokenizerPath    string
	ORTSharedLibPath string
	Timeout          time.Duration
}

type ONNXRuntimeGoEmbeddingService struct {
	modelPath     string
	tokenizerPath string
	timeout       time.Duration

	tokenizer *tokenizer.Tokenizer
	padID     int
	session   *ort.DynamicAdvancedSession

	tokenizerMu sync.Mutex
	sessionMu   sync.Mutex

	cache *embeddingLRU

	inflightMu sync.Mutex
	inflight   map[string][]chan embedResult

	closeOnce sync.Once
}

type embedResult struct {
	vector []float32
	err    error
}

func NewONNXRuntimeGoEmbeddingService(cfg ONNXRuntimeGoEmbeddingConfig) (*ONNXRuntimeGoEmbeddingService, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	modelPath := absIfRel(strings.TrimSpace(cfg.ModelPath))
	tokenizerPath := absIfRel(strings.TrimSpace(cfg.TokenizerPath))
	if modelPath == "" || tokenizerPath == "" {
		return nil, fmt.Errorf("onnx_go embedding requires model_path and tokenizer_path")
	}
	if strings.TrimSpace(cfg.ORTSharedLibPath) == "" {
		return nil, fmt.Errorf("onnx_go embedding requires ort shared library path")
	}
	if err := acquireORTEnvironment(cfg.ORTSharedLibPath); err != nil {
		return nil, err
	}

	tokFile, err := resolveTokenizerJSONPath(tokenizerPath)
	if err != nil {
		_ = releaseORTEnvironment()
		return nil, err
	}
	tok, err := pretrained.FromFile(tokFile)
	if err != nil {
		_ = releaseORTEnvironment()
		return nil, fmt.Errorf("load tokenizer failed: %w", err)
	}
	padID, ok := tok.TokenToId("<pad>")
	if !ok {
		padID = 1
	}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"token_embeddings"},
		nil,
	)
	if err != nil {
		_ = releaseORTEnvironment()
		return nil, fmt.Errorf("create onnx session failed: %w", err)
	}

	return &ONNXRuntimeGoEmbeddingService{
		modelPath:     modelPath,
		tokenizerPath: tokenizerPath,
		timeout:       timeout,
		tokenizer:     tok,
		padID:         padID,
		session:       session,
		cache:         newEmbeddingLRU(512),
		inflight:      make(map[string][]chan embedResult),
	}, nil
}

func (s *ONNXRuntimeGoEmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
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

func (s *ONNXRuntimeGoEmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	normalized := make([]string, 0, len(texts))
	for _, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, sanitizeUTF8(trimmed))
	}
	if len(normalized) == 0 {
		return [][]float32{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return nil, err
	}

	inputIDs, attentionMask, batchSize, seqLen, err := s.encodeBatch(normalized)
	if err != nil {
		return nil, err
	}
	if batchSize == 0 || seqLen == 0 {
		return [][]float32{}, nil
	}

	shape := ort.NewShape(int64(batchSize), int64(seqLen))
	inputTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor failed: %w", err)
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor failed: %w", err)
	}
	defer maskTensor.Destroy()

	outputs := []ort.Value{nil}
	s.sessionMu.Lock()
	err = s.session.Run([]ort.Value{inputTensor, maskTensor}, outputs)
	s.sessionMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("onnx session run failed: %w", err)
	}
	if len(outputs) == 0 || outputs[0] == nil {
		return nil, fmt.Errorf("onnx session output is empty")
	}
	defer outputs[0].Destroy()

	tokenEmbeddings, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output tensor type")
	}
	raw := tokenEmbeddings.GetData()
	outShape := tokenEmbeddings.GetShape()
	if len(outShape) != 3 {
		return nil, fmt.Errorf("unexpected output shape: %v", outShape)
	}
	hiddenSize := int(outShape[2])
	if hiddenSize <= 0 {
		return nil, fmt.Errorf("invalid hidden size: %d", hiddenSize)
	}
	return meanPoolNormalize(raw, attentionMask, batchSize, seqLen, hiddenSize), nil
}

func (s *ONNXRuntimeGoEmbeddingService) registerInflight(text string) chan embedResult {
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

func (s *ONNXRuntimeGoEmbeddingService) finishInflight(text string, vector []float32, err error) {
	s.inflightMu.Lock()
	waiters := s.inflight[text]
	delete(s.inflight, text)
	s.inflightMu.Unlock()
	for _, ch := range waiters {
		ch <- embedResult{vector: vector, err: err}
		close(ch)
	}
}

func (s *ONNXRuntimeGoEmbeddingService) getCachedVector(text string) ([]float32, bool) {
	return s.cache.get(text)
}

func (s *ONNXRuntimeGoEmbeddingService) setCachedVector(text string, vector []float32) {
	s.cache.put(text, vector)
}

func (s *ONNXRuntimeGoEmbeddingService) encodeBatch(texts []string) ([]int64, []int64, int, int, error) {
	s.tokenizerMu.Lock()
	defer s.tokenizerMu.Unlock()

	s.tokenizer.WithTruncation(&tokenizer.TruncationParams{
		MaxLength: 512,
		Strategy:  tokenizer.LongestFirst,
		Stride:    0,
	})
	s.tokenizer.WithPadding(&tokenizer.PaddingParams{
		Strategy:  *tokenizer.NewPaddingStrategy(tokenizer.WithBatchLongest()),
		Direction: tokenizer.Right,
		PadId:     s.padID,
		PadTypeId: 0,
		PadToken:  "<pad>",
	})

	inputs := make([]tokenizer.EncodeInput, 0, len(texts))
	for _, text := range texts {
		inputs = append(inputs, tokenizer.NewSingleEncodeInput(tokenizer.NewInputSequence(text)))
	}
	encodings, err := s.tokenizer.EncodeBatch(inputs, true)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("tokenize batch failed: %w", err)
	}
	if len(encodings) == 0 {
		return []int64{}, []int64{}, 0, 0, nil
	}

	seqLen := len(encodings[0].GetIds())
	if seqLen == 0 {
		return []int64{}, []int64{}, len(encodings), 0, nil
	}
	ids := make([]int64, 0, len(encodings)*seqLen)
	mask := make([]int64, 0, len(encodings)*seqLen)
	for _, encoding := range encodings {
		rowIDs := encoding.GetIds()
		rowMask := encoding.GetAttentionMask()
		if len(rowIDs) != seqLen || len(rowMask) != seqLen {
			return nil, nil, 0, 0, fmt.Errorf("unexpected tokenized length mismatch")
		}
		for i := 0; i < seqLen; i++ {
			ids = append(ids, int64(rowIDs[i]))
			mask = append(mask, int64(rowMask[i]))
		}
	}
	return ids, mask, len(encodings), seqLen, nil
}

func meanPoolNormalize(tokenEmbeddings []float32, attentionMask []int64, batchSize, seqLen, hiddenSize int) [][]float32 {
	out := make([][]float32, batchSize)
	for b := 0; b < batchSize; b++ {
		sum := make([]float64, hiddenSize)
		var count float64
		for t := 0; t < seqLen; t++ {
			mask := attentionMask[b*seqLen+t]
			if mask == 0 {
				continue
			}
			count += 1
			base := (b*seqLen + t) * hiddenSize
			for h := 0; h < hiddenSize; h++ {
				sum[h] += float64(tokenEmbeddings[base+h])
			}
		}
		if count < 1e-9 {
			count = 1e-9
		}
		vec := make([]float32, hiddenSize)
		var l2 float64
		for h := 0; h < hiddenSize; h++ {
			v := sum[h] / count
			l2 += v * v
			vec[h] = float32(v)
		}
		norm := math.Sqrt(l2)
		if norm < 1e-12 {
			norm = 1e-12
		}
		inv := float32(1.0 / norm)
		for h := 0; h < hiddenSize; h++ {
			vec[h] = vec[h] * inv
		}
		out[b] = vec
	}
	return out
}

func sanitizeUTF8(s string) string {
	// 与 Python 端替换非法代理对齐：遇到坏 UTF-8 以 replacement rune 代替。
	return strings.ToValidUTF8(s, "\uFFFD")
}

func resolveTokenizerJSONPath(tokenizerPath string) (string, error) {
	if isFile(tokenizerPath) {
		return tokenizerPath, nil
	}
	tokFile := filepath.Join(tokenizerPath, "tokenizer.json")
	if isFile(tokFile) {
		return tokFile, nil
	}
	return "", fmt.Errorf("tokenizer.json not found under %q", tokenizerPath)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func absIfRel(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Join(wd, filepath.Clean(path))
}

func (s *ONNXRuntimeGoEmbeddingService) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.sessionMu.Lock()
		if s.session != nil {
			if err := s.session.Destroy(); err != nil {
				closeErr = err
			}
			s.session = nil
		}
		s.sessionMu.Unlock()
		if err := releaseORTEnvironment(); closeErr == nil && err != nil {
			closeErr = err
		}
	})
	return closeErr
}
