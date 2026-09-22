package raster

import (
	"math/rand"
	"slices"
	"testing"
)

func TestSetCoverageRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, width := range []int{1, 16, 257, 65535, 65536, 131071} {
		runs := NewAlphaRuns(width)
		coverage := make([]uint8, width)
		for _, random := range []bool{true, false} {
			for i := range coverage {
				coverage[i] = 0
				if random {
					coverage[i] = uint8(rng.Intn(256))
				}
			}
			runs.SetCoverage(coverage)
			got := make([]uint8, width)
			for x, alpha := range runs.Iter() {
				got[x] = alpha
			}
			if !slices.Equal(got, coverage) {
				t.Fatalf("coverage differs at width=%d, random=%v", width, random)
			}
		}
	}
}
