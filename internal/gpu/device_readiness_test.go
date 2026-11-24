// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gg"
)

// Having a device object is not sufficient to accept a draw: device setup
// must be complete. Conversely, rasterAtlas needs no shape pipelines.
// Use the noop backend so both readiness states run on every machine.
func TestDrawOperationsRequireReadyDevice(t *testing.T) {
	for _, ready := range []bool{false, true} {
		state := "not_ready"
		if ready {
			state = "raster_atlas_ready"
		}
		for _, operation := range []string{"fill_path", "stroke_path", "fill_shape", "stroke_shape"} {
			t.Run(state+"/"+operation, func(t *testing.T) {
				device, queue, cleanup := createNoopDevice(t)
				t.Cleanup(cleanup)
				s := NewGPUShared()
				s.device = device
				s.queue = queue
				s.deviceReady = ready
				s.strategy = strategyRasterAtlas
				rc := s.NewRenderContext()
				t.Cleanup(rc.Close)
				rc.pipelineMode = gg.PipelineModeRenderPass
				paint := gg.NewPaint()
				paint.SetStroke(gg.Stroke{Width: 3})
				path := gg.NewPath()
				path.MoveTo(5, 35)
				path.LineTo(30, 5)
				path.LineTo(55, 35)
				path.Close()
				shape := gg.DetectedShape{Kind: gg.ShapeRect, CenterX: 30, CenterY: 20, Width: 40, Height: 20}
				target := makeTestTarget(60, 40)
				var err error
				switch operation {
				case "fill_path":
					err = rc.FillPath(target, path, paint)
				case "stroke_path":
					err = rc.StrokePath(target, path, paint)
				case "fill_shape":
					err = rc.FillShape(target, shape, paint)
				case "stroke_shape":
					err = rc.StrokeShape(target, shape, paint)
				}
				if !ready {
					if !errors.Is(err, gg.ErrFallbackToCPU) {
						t.Errorf("draw returned %v, want ErrFallbackToCPU", err)
					}
					if len(rc.pendingDraws) != 0 || rc.hasPendingTarget {
						t.Error("draw queued before device setup completed")
					}
					return
				}
				if err != nil {
					t.Errorf("draw returned %v with a ready rasterAtlas device", err)
				}
				if len(rc.pendingDraws) != 1 || !rc.hasPendingTarget {
					t.Error("draw was not queued with a ready rasterAtlas device")
				}
			})
		}
	}
}
