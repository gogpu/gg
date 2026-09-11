package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// TestStrokePath_SelfOverlapMatchesCPUStroker verifies that a stroke which
// overlaps itself renders through the GPU render context's queue exactly as
// SoftwareRenderer.Stroke renders it.
//
// preTessellateStroke expands the stroke once, at queue time, and the result
// is filled — on the GPU, or through dispatchDrawsToSoftware on a software
// adapter. The fill rule is what decides the overlaps: a tight zigzag's
// consecutive segments cover the same pixels near every turn, and an EvenOdd
// fill cancels those pixels into holes, while the CPU stroker fills the same
// expander output NonZero and covers them. Comparing the two pixel by pixel is
// what catches a fill rule that does not match, without needing a GPU.
func TestStrokePath_SelfOverlapMatchesCPUStroker(t *testing.T) {
	const size = 200

	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	// A tight zigzag: segments 22 px wide, turning back every 16 px, so each
	// turn is covered twice by the expanded outline.
	zigzag := func() *gg.Path {
		p := gg.NewPath()
		for k := 0; k <= 10; k++ {
			x := 20 + 16*float64(k)
			y := 30.0
			if k%2 == 1 {
				y = 170
			}
			if k == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		return p
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 22, Cap: gg.LineCapButt, Join: gg.LineJoinMiter, MiterLimit: 10})

	// Through the render context's queue.
	target := makeTestTarget(size, size)
	if err := rc.StrokePath(target, zigzag(), paint); err != nil {
		t.Fatalf("StrokePath: %v", err)
	}
	pmQueue := gg.NewPixmap(size, size)
	rc.dispatchDrawsToSoftware(pmQueue, gg.NewSoftwareRenderer(size, size))

	// The CPU stroker, which is the reference.
	pmRef := gg.NewPixmap(size, size)
	if err := gg.NewSoftwareRenderer(size, size).Stroke(pmRef, zigzag(), paint); err != nil {
		t.Fatalf("reference Stroke: %v", err)
	}

	queue, ref := pmQueue.Data(), pmRef.Data()
	inked, wrong := 0, 0
	for i := 3; i < len(ref); i += 4 {
		if ref[i] > 128 {
			inked++
		}
		d := int(queue[i]) - int(ref[i])
		if d > 64 || d < -64 {
			wrong++
		}
	}
	if inked == 0 {
		t.Fatal("the reference stroke drew nothing, so the comparison says nothing")
	}
	if wrong > inked/100 {
		t.Errorf("%d of %d stroked pixels differ from the CPU stroker by more than a quarter of full "+
			"coverage: the overlaps of a self-overlapping stroke are being canceled", wrong, inked)
	}
}
