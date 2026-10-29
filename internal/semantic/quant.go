package semantic

import (
	"math"
	"math/bits"
)

// Quantization stores a vector in fewer bits: each of a unit vector's
// numbers becomes one signed byte (int8, -127..127) times one float scale
// per vector, the largest absolute value / 127. That's a quarter of
// float32's memory (300k × 768: 230 MB, not 920 MB), and on this
// lexicon's real vectors the 10 nearest neighbours stay 99% the same.
//
// The query stays float32 ("asymmetric" scoring): only the stored side
// loses precision, and the score is scale × Σ q·code.
//
// Sign bits go further: one bit per number, whether it is positive. Two
// vectors' Hamming distance (how many signs differ, a popcount of their
// XOR) roughly tracks the angle between them, at 64 numbers per machine
// instruction. Too rough to rank by (58% of the neighbours), but good to
// cut 300k candidates to a few hundred, which int8 then ranks: a
// first-pass filter and a rescore, as vector databases do.

// quantize returns v's int8 codes and scale; v ≈ codes × scale. A zero
// vector gets scale 0.
func quantize(v []float32) ([]int8, float32) {
	var m float32
	for _, x := range v {
		m = max(m, float32(math.Abs(float64(x))))
	}
	codes := make([]int8, len(v))
	if m == 0 {
		return codes, 0
	}
	scale := m / 127
	for i, x := range v {
		codes[i] = int8(math.Round(float64(x / scale)))
	}
	return codes, scale
}

// dequantize is quantize's inverse, up to its rounding.
func dequantize(codes []int8, scale float32) []float32 {
	out := make([]float32, len(codes))
	for i, c := range codes {
		out[i] = float32(c) * scale
	}
	return out
}

// dotInt8 is the asymmetric score: q · (codes × scale).
func dotInt8(q []float32, codes []int8, scale float32) float64 {
	var s float32
	for i, c := range codes[:len(q)] {
		s += q[i] * float32(c)
	}
	return float64(s * scale)
}

// words is the number of uint64 that hold dims sign bits.
func words(dims int) int { return (dims + 63) / 64 }

// signsOf sets bit i of out when v[i] > 0.
func signsOf[T int8 | float32](v []T, out []uint64) {
	clear(out)
	for i, x := range v {
		if x > 0 {
			out[i/64] |= 1 << (i % 64)
		}
	}
}

// hamming counts the bits that differ.
func hamming(a, b []uint64) int {
	n := 0
	for i := range a {
		n += bits.OnesCount64(a[i] ^ b[i])
	}
	return n
}

// truncate keeps v's first dims numbers and scales them back to length 1:
// Matryoshka embeddings are trained so a prefix is still an embedding.
// On this lexicon nomic-embed-text keeps only 84% of the neighbours at 512
// and 73% at 256, so the default is all of them.
func truncate(v []float32, dims int) []float32 {
	if dims <= 0 || dims >= len(v) {
		return normalize(v)
	}
	return normalize(v[:dims])
}
