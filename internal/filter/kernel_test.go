package filter

import (
	"math"
	"testing"
)

func TestGaussianKernelZeroRadius(t *testing.T) {
	kernel := GaussianKernel(0)

	if len(kernel) != 1 {
		t.Errorf("GaussianKernel(0) len = %d, want 1", len(kernel))
	}

	if kernel[0] != 1.0 {
		t.Errorf("GaussianKernel(0)[0] = %v, want 1.0", kernel[0])
	}
}

func TestGaussianKernelNegativeRadius(t *testing.T) {
	kernel := GaussianKernel(-5)

	if len(kernel) != 1 {
		t.Errorf("GaussianKernel(-5) len = %d, want 1", len(kernel))
	}
}

func TestGaussianKernelNormalized(t *testing.T) {
	radii := []float64{1, 2, 3, 5, 10, 20}

	for _, r := range radii {
		kernel := GaussianKernel(r)

		// Sum should be very close to 1.0
		var sum float32
		for _, v := range kernel {
			sum += v
		}

		if math.Abs(float64(sum)-1.0) > 0.001 {
			t.Errorf("GaussianKernel(%v) sum = %v, want ~1.0", r, sum)
		}
	}
}

func TestGaussianKernelSymmetric(t *testing.T) {
	kernel := GaussianKernel(5)
	n := len(kernel)

	for i := 0; i < n/2; i++ {
		j := n - 1 - i
		if math.Abs(float64(kernel[i]-kernel[j])) > 0.0001 {
			t.Errorf("kernel[%d] = %v != kernel[%d] = %v (asymmetric)", i, kernel[i], j, kernel[j])
		}
	}
}

func TestGaussianKernelSize(t *testing.T) {
	tests := []struct {
		radius   float64
		wantSize int
	}{
		{0.5, 5},   // ceil(0.5*3)*2+1 = 2*2+1 = 5
		{1.0, 7},   // ceil(1*3)*2+1 = 3*2+1 = 7
		{2.0, 13},  // ceil(2*3)*2+1 = 6*2+1 = 13
		{5.0, 31},  // ceil(5*3)*2+1 = 15*2+1 = 31
		{10.0, 61}, // ceil(10*3)*2+1 = 30*2+1 = 61
	}

	for _, tt := range tests {
		kernel := GaussianKernel(tt.radius)
		if len(kernel) != tt.wantSize {
			t.Errorf("GaussianKernel(%v) len = %d, want %d", tt.radius, len(kernel), tt.wantSize)
		}
	}
}

func TestGaussianKernelPeakAtCenter(t *testing.T) {
	kernel := GaussianKernel(5)
	center := len(kernel) / 2

	// Center should be the maximum
	maxIdx := 0
	maxVal := kernel[0]
	for i, v := range kernel {
		if v > maxVal {
			maxVal = v
			maxIdx = i
		}
	}

	if maxIdx != center {
		t.Errorf("kernel peak at %d, want %d (center)", maxIdx, center)
	}
}

func TestBoxKernelZeroRadius(t *testing.T) {
	kernel := BoxKernel(0)

	if len(kernel) != 1 {
		t.Errorf("BoxKernel(0) len = %d, want 1", len(kernel))
	}

	if kernel[0] != 1.0 {
		t.Errorf("BoxKernel(0)[0] = %v, want 1.0", kernel[0])
	}
}

func TestBoxKernelUniform(t *testing.T) {
	kernel := BoxKernel(3)
	expectedSize := 7 // 3*2+1
	expectedVal := float32(1.0 / 7.0)

	if len(kernel) != expectedSize {
		t.Errorf("BoxKernel(3) len = %d, want %d", len(kernel), expectedSize)
	}

	for i, v := range kernel {
		if math.Abs(float64(v-expectedVal)) > 0.0001 {
			t.Errorf("BoxKernel(3)[%d] = %v, want %v", i, v, expectedVal)
		}
	}
}

func TestBoxKernelNormalized(t *testing.T) {
	radii := []int{1, 2, 5, 10}

	for _, r := range radii {
		kernel := BoxKernel(r)

		var sum float32
		for _, v := range kernel {
			sum += v
		}

		if math.Abs(float64(sum)-1.0) > 0.001 {
			t.Errorf("BoxKernel(%d) sum = %v, want ~1.0", r, sum)
		}
	}
}

func TestCachedGaussianKernel(t *testing.T) {
	// First call should generate and cache
	kernel1 := CachedGaussianKernel(5.0)

	// Second call should return cached
	kernel2 := CachedGaussianKernel(5.0)

	// Should be same length and values
	if len(kernel1) != len(kernel2) {
		t.Errorf("cached kernel len mismatch: %d != %d", len(kernel1), len(kernel2))
	}

	for i := range kernel1 {
		if kernel1[i] != kernel2[i] {
			t.Errorf("cached kernel[%d] mismatch: %v != %v", i, kernel1[i], kernel2[i])
		}
	}
}

func TestCachedGaussianKernelDifferentRadii(t *testing.T) {
	k1 := CachedGaussianKernel(5.0)
	k2 := CachedGaussianKernel(10.0)

	if len(k1) == len(k2) {
		t.Error("different radii should produce different kernel sizes")
	}
}

func TestOptimalKernelSize(t *testing.T) {
	tests := []struct {
		radius float64
		want   int
	}{
		{0, 1},
		{-1, 1},
		{1.0, 7},
		{5.0, 31},
	}

	for _, tt := range tests {
		got := OptimalKernelSize(tt.radius)
		if got != tt.want {
			t.Errorf("OptimalKernelSize(%v) = %d, want %d", tt.radius, got, tt.want)
		}
	}
}

func TestKernelCenter(t *testing.T) {
	tests := []struct {
		size int
		want int
	}{
		{1, 0},
		{3, 1},
		{5, 2},
		{7, 3},
		{31, 15},
	}

	for _, tt := range tests {
		got := KernelCenter(tt.size)
		if got != tt.want {
			t.Errorf("KernelCenter(%d) = %d, want %d", tt.size, got, tt.want)
		}
	}
}

func BenchmarkGaussianKernel(b *testing.B) {
	radii := []float64{1, 5, 10, 20}

	for _, r := range radii {
		b.Run(fmtRadius(r), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = GaussianKernel(r)
			}
		})
	}
}

func BenchmarkCachedGaussianKernel(b *testing.B) {
	radii := []float64{1, 5, 10, 20}

	for _, r := range radii {
		b.Run(fmtRadius(r), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = CachedGaussianKernel(r)
			}
		})
	}
}

func fmtRadius(r float64) string {
	return "r=" + formatFloat(r)
}
