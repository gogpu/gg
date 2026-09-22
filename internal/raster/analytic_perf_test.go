package raster

import (
	"math/rand"
	"slices"
	"testing"
)

func TestResolvedEdgesSortMatchesInsertion(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	af := NewAnalyticFiller(100, 100)
	for n := 0; n < 300; n++ {
		edges := make([]edgeLineState, n)
		for i := range edges {
			edges[i] = edgeLineState{topX: int32(rng.Intn(8)), botX: int32(rng.Intn(8)), dy: int32(i)}
		}
		want := slices.Clone(edges)
		sortEdgesByTopX(want)
		af.resolvedEdges = edges
		af.sortResolvedEdges()
		if !slices.Equal(edges, want) {
			t.Fatalf("sort differs at n=%d (including stability of ties)", n)
		}
	}
}

func TestEdgesCrossMatchesPairs(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for n := 0; n < 150; n++ {
		for trial := 0; trial < 10; trial++ {
			spans := make([]edgeSpan, n)
			for i := range spans {
				x := int32(rng.Intn(20) - 10)
				y := x
				if trial%2 == 0 {
					y = int32(rng.Intn(20) - 10)
				}
				spans[i] = edgeSpan{x, y}
			}
			want := false
			for i, a := range spans {
				for _, b := range spans[i+1:] {
					if (a.topX < b.topX && a.botX > b.botX) || (a.topX > b.topX && a.botX < b.botX) {
						want = true
					}
				}
			}
			if got := edgesCross(spans); got != want {
				t.Fatalf("n=%d trial=%d: got %v, want %v", n, trial, got, want)
			}
		}
	}
}

func TestSmallEdgeCrossingDoesNotAllocate(t *testing.T) {
	eb := NewEdgeBuilder(2)
	eb.BuildFromPath(makeCirclePath(20, 20, 10), IdentityTransform{})
	af := NewAnalyticFiller(40, 40)
	for _, e := range eb.sortedEdgesSlice()[:16] {
		af.aet.Insert(e.variant)
	}
	if af.aet.Len() < 2 || af.aet.Len() > 16 {
		t.Fatalf("unexpected edge count: %d", af.aet.Len())
	}
	allocs := testing.AllocsPerRun(100, func() {
		af.hasEdgeCrossing(20*skFixed1, 21*skFixed1, 4)
	})
	if allocs != 0 {
		t.Fatalf("small crossing check allocated %g times", allocs)
	}
}

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
