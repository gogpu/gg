// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// Empty paths must be no-ops even before a device has been initialized.
// In particular, they must not request a CPU fallback or queue an empty draw.
func TestEmptyPathsDoNotInitializeDevice(t *testing.T) {
	for _, mode := range []gg.PipelineMode{gg.PipelineModeAuto, gg.PipelineModeRenderPass, gg.PipelineModeCompute} {
		for _, operation := range []string{"fill", "stroke", "dashed_stroke"} {
			t.Run(mode.String()+"/"+operation, func(t *testing.T) {
				s := NewGPUShared()
				t.Cleanup(s.Close)
				rc := s.NewRenderContext()
				t.Cleanup(rc.Close)
				rc.pipelineMode = mode
				paint := gg.NewPaint()
				if operation == "dashed_stroke" {
					paint.SetStroke(gg.DashedStroke(2, 2))
				}
				path := gg.NewPath()
				target := makeTestTarget(60, 40)
				var err error
				if operation == "fill" {
					err = rc.FillPath(target, path, paint)
				} else {
					err = rc.StrokePath(target, path, paint)
				}
				if err != nil {
					t.Errorf("empty path returned %v, want nil", err)
				}
				if s.instance != nil || s.device != nil || s.deviceReady {
					t.Error("empty path initialized GPU resources")
				}
				if len(rc.pendingDraws) != 0 || rc.hasPendingTarget {
					t.Error("empty path queued work")
				}
			})
		}
	}
}
