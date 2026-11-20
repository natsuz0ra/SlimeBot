package inference

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"

	"gonum.org/v1/gonum/blas"
	"gonum.org/v1/gonum/blas/gonum"
)

const width = 384
const intermediate = 1536
const heads = 12
const labels = 21

type normalization struct{ weight, bias []float32 }
type layer struct {
	qkv, attention, up, down                         linear
	qkvBias, attentionBias, upBias, downBias, smooth []float32
	attentionNorm, outputNorm                        normalization
}

// Model is immutable after loading and safe to use from multiple goroutines.
// Callers limit concurrency to bound scratch-memory and CPU consumption.
type Model struct {
	tokenizer               *Tokenizer
	words, positions, types embedding
	norm                    normalization
	layers                  []layer
	classifier              linear
	bias                    []float32
}

func Load(ctx context.Context, dir string) (*Model, error) {
	g, err := readGraph(dir)
	if err != nil {
		return nil, err
	}
	m := &Model{}
	var failure error
	f := func(name string, n int) []float32 {
		x, err := g.floats(name, n)
		if err != nil {
			failure = fmt.Errorf("%w: %s", errModel, name)
		}
		return x
	}
	e := func(name string, n int) embedding {
		x, err := g.embedding(name, n, width)
		if err != nil {
			failure = errModel
		}
		return x
	}
	l := func(name string, in, out int) linear {
		x, err := g.linear(name, in, out)
		if err != nil {
			failure = fmt.Errorf("%w: %s", errModel, name)
		}
		return x
	}
	n := func(name string) normalization {
		return normalization{f(name+".weight", width), f(name+".bias", width)}
	}
	m.words = e("inner.bert.embeddings.word_embeddings.weight", 40003)
	m.positions = e("inner.bert.embeddings.position_embeddings.weight", 512)
	m.types = e("inner.bert.embeddings.token_type_embeddings.weight", 2)
	m.norm = n("inner.bert.embeddings.LayerNorm")
	for i := 0; i < 12; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		prefix := fmt.Sprintf("inner.bert.encoder.layer.%d.", i)
		matrix := func(part string, in, out int) linear {
			return l(g.matrices[fmt.Sprintf("/inner/bert/encoder/layer.%d/%s/dense/MatMul_quant", i, part)], in, out)
		}
		m.layers = append(m.layers, layer{
			qkv:       l(fmt.Sprintf("Attention_%d_qkv_weight", i), width, 3*width),
			attention: matrix("attention/output", width, width), up: matrix("intermediate", width, intermediate), down: matrix("output", intermediate, width),
			qkvBias:       f(fmt.Sprintf("Attention_%d_qkv_bias", i), 3*width),
			attentionBias: f(prefix+"attention.output.dense.bias", width), upBias: f(prefix+"intermediate.dense.bias", intermediate), downBias: f(prefix+"output.dense.bias", width),
			smooth:        f(fmt.Sprintf("r15_smooth_inv_s_%d", i), intermediate),
			attentionNorm: n(prefix + "attention.output.LayerNorm"), outputNorm: n(prefix + "output.LayerNorm"),
		})
		if failure != nil {
			return nil, failure
		}
	}
	m.classifier = l(g.matrices["/inner/classifier/MatMul_quant"], width, labels)
	m.bias = f("inner.classifier.bias", labels)
	if failure != nil {
		return nil, failure
	}
	m.tokenizer, err = LoadTokenizer(dir)
	if err != nil {
		return nil, err
	}
	return m, ctx.Err()
}

func (e embedding) add(dst []float32, row int) {
	for j := range e.scale {
		dst[j] += (float32(e.data[row*width+j]) - float32(e.zero[j])) * e.scale[j]
	}
}

func (n normalization) apply(x, residual, bias []float32) {
	for row := 0; row < len(x)/width; row++ {
		y := x[row*width : (row+1)*width]
		var sum float64
		for j := range y {
			if bias != nil {
				y[j] += bias[j]
			}
			if residual != nil {
				y[j] += residual[row*width+j]
			}
			sum += float64(y[j])
		}
		mean := float32(sum / width)
		var variance float64
		for _, v := range y {
			d := v - mean
			variance += float64(d * d)
		}
		inv := float32(1 / math.Sqrt(variance/width+1e-12))
		for j, v := range y {
			y[j] = (v-mean)*inv*n.weight[j] + n.bias[j]
		}
	}
}

func quantize(x []float32) ([]float32, float32) {
	var lo, hi float32
	for _, v := range x {
		lo = min(lo, v)
		hi = max(hi, v)
	}
	scale := (hi - lo) / 255
	if scale == 0 {
		scale = 1
	}
	zero := float32(math.RoundToEven(float64(-lo / scale)))
	q := make([]float32, len(x))
	for i, v := range x {
		q[i] = max(0, min(255, float32(math.RoundToEven(float64(v/scale)))+zero)) - zero
	}
	return q, scale
}

func (l linear) apply(ctx context.Context, x []float32) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q, scale := quantize(x)
	rows := len(x) / l.in
	y := make([]float32, rows*l.out)
	// Small row blocks keep cancellation responsive even on the scalar fallback.
	columns := (l.out + 127) / 128
	err := parallel(ctx, (rows+7)/8*columns, func(block int) {
		start := block / columns * 8
		end := min(start+8, rows)
		column := block % columns * 128
		count := min(128, l.out-column)
		gonum.Implementation{}.Sgemm(blas.NoTrans, blas.Trans, end-start, count, l.in, 1, q[start*l.in:end*l.in], l.in, l.weights[column*l.in:], l.in, 0, y[start*l.out+column:], l.out)
	})
	if err != nil {
		return nil, err
	}
	for i := range y {
		y[i] *= scale * l.scale[i%l.out]
	}
	return y, nil
}

func parallel(ctx context.Context, count int, work func(int)) error {
	workers := min(count, min(4, runtime.GOMAXPROCS(0)))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; i < count; i += workers {
				if ctx.Err() != nil {
					return
				}
				work(i)
			}
		}(w)
	}
	wg.Wait()
	return ctx.Err()
}

func attention(ctx context.Context, qkv []float32, rows int) ([]float32, error) {
	result := make([]float32, rows*width)
	const headWidth = width / heads
	err := parallel(ctx, heads, func(h int) {
		scores := make([]float32, rows*rows)
		for i := 0; i < rows; i++ {
			var maximum float32 = -math.MaxFloat32
			for j := 0; j < rows; j++ {
				var dot float32
				for d := 0; d < headWidth; d++ {
					dot += qkv[i*3*width+h*headWidth+d] * qkv[j*3*width+width+h*headWidth+d]
				}
				dot /= float32(math.Sqrt(headWidth))
				scores[i*rows+j] = dot
				maximum = max(maximum, dot)
			}
			var sum float32
			for j := 0; j < rows; j++ {
				v := float32(math.Exp(float64(scores[i*rows+j] - maximum)))
				scores[i*rows+j] = v
				sum += v
			}
			for j := 0; j < rows; j++ {
				scores[i*rows+j] /= sum
			}
		}
		// V is a strided view of the fused Q/K/V projection.
		gonum.Implementation{}.Sgemm(blas.NoTrans, blas.NoTrans, rows, headWidth, rows, 1, scores, rows, qkv[2*width+h*headWidth:], 3*width, 0, result[h*headWidth:], width)
	})
	return result, err
}

// Logits returns row-major token classification scores for one unpadded window.
func (m *Model) Logits(ctx context.Context, ids []int) ([]float32, error) {
	if len(ids) < 2 || len(ids) > 512 {
		return nil, errModel
	}
	x := make([]float32, len(ids)*width)
	for i, id := range ids {
		if id < 0 || id >= m.words.rows {
			return nil, errModel
		}
		row := x[i*width : (i+1)*width]
		m.words.add(row, id)
		m.types.add(row, 0)
		m.positions.add(row, i)
	}
	m.norm.apply(x, nil, nil)
	for _, l := range m.layers {
		qkv, err := l.qkv.apply(ctx, x)
		if err != nil {
			return nil, err
		}
		for i := range qkv {
			qkv[i] += l.qkvBias[i%(3*width)]
		}
		a, err := attention(ctx, qkv, len(ids))
		if err != nil {
			return nil, err
		}
		a, err = l.attention.apply(ctx, a)
		if err != nil {
			return nil, err
		}
		l.attentionNorm.apply(a, x, l.attentionBias)
		u, err := l.up.apply(ctx, a)
		if err != nil {
			return nil, err
		}
		for i := range u {
			v := u[i] + l.upBias[i%intermediate]
			u[i] = 0.5 * v * (1 + float32(math.Erf(float64(v)/math.Sqrt2))) * l.smooth[i%intermediate]
		}
		x, err = l.down.apply(ctx, u)
		if err != nil {
			return nil, err
		}
		l.outputNorm.apply(x, a, l.downBias)
	}
	y, err := m.classifier.apply(ctx, x)
	if err != nil {
		return nil, err
	}
	for i := range y {
		y[i] += m.bias[i%labels]
	}
	return y, nil
}
