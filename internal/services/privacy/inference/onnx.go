// Package inference implements the fixed AstrLink Guard INT8 encoder in Go.
// It reads publisher weights; it is not a general-purpose ONNX executor.
package inference

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var errModel = errors.New("unsupported or damaged privacy model")

type field struct {
	number, wire int
	value        uint64
	data         []byte
}

// walk decodes the small subset of protobuf wire types used by ONNX. Every
// length and offset is checked before slicing, including unknown fields.
func walk(data []byte, visit func(field) error) error {
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 || tag>>3 == 0 {
			return errModel
		}
		data = data[n:]
		f := field{number: int(tag >> 3), wire: int(tag & 7)}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return errModel
			}
			f.value = v
			data = data[n:]
		case 1, 5:
			n := 8
			if f.wire == 5 {
				n = 4
			}
			if len(data) < n {
				return errModel
			}
			f.data = data[:n]
			data = data[n:]
		case 2:
			v, n := binary.Uvarint(data)
			if n <= 0 || v > uint64(len(data)-n) {
				return errModel
			}
			data = data[n:]
			f.data = data[:int(v)]
			data = data[int(v):]
		default:
			return errModel
		}
		if err := visit(f); err != nil {
			return err
		}
	}
	return nil
}

type tensor struct {
	dims []int
	kind int
	raw  []byte
}
type graph struct {
	tensors  map[string]tensor
	matrices map[string]string
}

func readGraph(dir string) (*graph, error) {
	data, err := os.ReadFile(filepath.Join(dir, "model_int8.onnx"))
	if err != nil || len(data) > 1<<20 {
		return nil, errModel
	}
	weights, err := os.ReadFile(filepath.Join(dir, "model_int8.onnx_data"))
	if err != nil || len(weights) > 64<<20 {
		return nil, errModel
	}
	g := &graph{map[string]tensor{}, map[string]string{}}
	found := false
	err = walk(data, func(f field) error {
		if f.number != 7 {
			return nil
		}
		found = true
		return walk(f.data, func(f field) error {
			switch f.number {
			case 5:
				name, t, err := readTensor(f.data, weights)
				if err != nil {
					return fmt.Errorf("%w: tensor %s", err, name)
				}
				if _, ok := g.tensors[name]; ok {
					return errModel
				}
				g.tensors[name] = t
			case 1:
				var name, op string
				var inputs []string
				err := walk(f.data, func(f field) error {
					switch f.number {
					case 1:
						inputs = append(inputs, string(f.data))
					case 3:
						name = string(f.data)
					case 4:
						op = string(f.data)
					}
					return nil
				})
				if err != nil {
					return err
				}
				if op == "MatMulInteger" && len(inputs) == 4 {
					g.matrices[name] = inputs[1]
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	if !found || len(g.tensors) > 1024 {
		return nil, errModel
	}
	return g, nil
}

func readTensor(data, weights []byte) (string, tensor, error) {
	var t tensor
	var name string
	external := map[string]string{}
	err := walk(data, func(f field) error {
		switch f.number {
		case 1:
			if f.wire == 0 {
				if f.value > 1<<20 {
					return errModel
				}
				t.dims = append(t.dims, int(f.value))
			} else if f.wire == 2 {
				for b := f.data; len(b) > 0; {
					v, n := binary.Uvarint(b)
					if n <= 0 || v > 1<<20 {
						return errModel
					}
					t.dims = append(t.dims, int(v))
					b = b[n:]
				}
			} else {
				return errModel
			}
		case 2:
			t.kind = int(f.value)
		case 4: // float_data may be packed or scalar.
			if f.wire != 2 && f.wire != 5 {
				return errModel
			}
			t.raw = append(t.raw, f.data...)
		case 5, 7:
			appendValue := func(v uint64) {
				if f.number == 7 {
					t.raw = binary.LittleEndian.AppendUint64(t.raw, v)
				} else if t.kind == 2 || t.kind == 3 {
					t.raw = append(t.raw, byte(v))
				} else {
					t.raw = binary.LittleEndian.AppendUint32(t.raw, uint32(v))
				}
			}
			if f.wire == 0 {
				appendValue(f.value)
			} else if f.wire == 2 {
				for b := f.data; len(b) > 0; {
					v, n := binary.Uvarint(b)
					if n <= 0 {
						return errModel
					}
					appendValue(v)
					b = b[n:]
				}
			} else {
				return errModel
			}

		case 8:
			name = string(f.data)
		case 9:
			t.raw = f.data
		case 13:
			var key, value string
			if err := walk(f.data, func(f field) error {
				if f.number == 1 {
					key = string(f.data)
				}
				if f.number == 2 {
					value = string(f.data)
				}
				return nil
			}); err != nil {
				return err
			}
			external[key] = value
		}
		return nil
	})
	if err != nil || name == "" || len(t.dims) > 4 {
		return name, t, errModel
	}
	if len(external) > 0 {
		if external["location"] != "model_int8.onnx_data" {
			return name, t, errModel
		}
		o, e1 := strconv.ParseUint(external["offset"], 10, 64)
		n, e2 := strconv.ParseUint(external["length"], 10, 64)
		if e1 != nil || e2 != nil || o > uint64(len(weights)) || n > uint64(len(weights))-o {
			return name, t, errModel
		}
		t.raw = weights[int(o):int(o+n)]
	}
	count := 1
	for _, d := range t.dims {
		if d <= 0 || count > 64<<20/d {
			return name, t, errModel
		}
		count *= d
	}
	size := 0
	switch t.kind {
	case 1:
		size = 4
	case 2, 3:
		size = 1
	case 7:
		size = 8
	default:
		return name, t, errModel
	}
	if len(t.raw) != count*size {
		return name, t, errModel
	}
	return name, t, nil
}

func (g *graph) floats(name string, n int) ([]float32, error) {
	t, ok := g.tensors[name]
	if !ok || t.kind != 1 || len(t.raw) != n*4 {
		return nil, errModel
	}
	x := make([]float32, n)
	for i := range x {
		x[i] = math.Float32frombits(binary.LittleEndian.Uint32(t.raw[i*4:]))
		if math.IsNaN(float64(x[i])) || math.IsInf(float64(x[i]), 0) {
			return nil, errModel
		}
	}
	return x, nil
}

type embedding struct {
	data  []byte
	scale []float32
	zero  []byte
	rows  int
}

func (g *graph) embedding(name string, rows, width int) (embedding, error) {
	t, ok := g.tensors[name+"_quantized"]
	z := g.tensors[name+"_zero_point"]
	if !ok || t.kind != 2 || len(t.dims) != 2 || t.dims[0] != rows || t.dims[1] != width || z.kind != 2 || len(z.raw) != width {
		return embedding{}, errModel
	}
	s, err := g.floats(name+"_scale", width)
	// Do not keep the entire external-data file alive through an embedding view.
	return embedding{slices.Clone(t.raw), s, slices.Clone(z.raw), rows}, err
}

// Keeping integer-valued operands as float32 lets the pure-Go BLAS kernels
// accelerate products without a shared library or Cgo. We center both operands
// before multiplication, and apply quantization scales afterwards.
type linear struct {
	weights, scale []float32
	in, out        int
}

func (g *graph) linear(name string, in, out int) (linear, error) {
	base := strings.TrimSuffix(name, "_quantized")
	t, ok := g.tensors[base+"_quantized"]
	z := g.tensors[base+"_zero_point"]
	if !ok || t.kind != 3 || len(t.dims) != 2 || t.dims[0] != in || t.dims[1] != out || z.kind != 3 || (len(z.raw) != out && len(z.raw) != 1) {
		return linear{}, errModel
	}
	scale, err := g.floats(base+"_scale", out)
	if err != nil {
		return linear{}, err
	}
	// Transpose once on load: each output channel is a contiguous dot product.
	w := make([]float32, in*out)
	for k := 0; k < in; k++ {
		for j := 0; j < out; j++ {
			w[j*in+k] = float32(int(int8(t.raw[k*out+j])) - int(int8(z.raw[j%len(z.raw)])))
		}
	}
	return linear{w, scale, in, out}, nil
}
