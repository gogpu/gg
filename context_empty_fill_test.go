package gg_test

import (
	"testing"

	"github.com/gogpu/gg"
)

func TestFillEmptyDimensions(t *testing.T) {
	for _, size := range [][2]int{{0, 20}, {20, 0}, {0, 0}} {
		dc := gg.NewContext(size[0], size[1])
		dc.MoveTo(0, 0)
		dc.LineTo(10, 10)
		dc.LineTo(0, 15)
		dc.ClosePath()
		if err := dc.Fill(); err != nil {
			t.Fatalf("Fill(%v): %v", size, err)
		}
	}
}
