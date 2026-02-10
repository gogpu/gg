//go:build !nogpu

// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package gpu

import (
	"github.com/gogpu/gg/internal/raster"
	"github.com/gogpu/gg/scene"
)

// scenePathAdapter wraps scene.Path to implement raster.PathLike.
type scenePathAdapter struct {
	path *scene.Path
}

func (a *scenePathAdapter) IsEmpty() bool { return a.path.IsEmpty() }

func (a *scenePathAdapter) Points() []float32 { return a.path.Points() }

func (a *scenePathAdapter) Verbs() []raster.PathVerb {
	sv := a.path.Verbs()
	rv := make([]raster.PathVerb, len(sv))
	for i, v := range sv {
		rv[i] = raster.PathVerb(v)
	}
	return rv
}

// sceneAffineAdapter wraps scene.Affine to implement raster.Transform.
type sceneAffineAdapter struct {
	affine scene.Affine
}

func (a sceneAffineAdapter) TransformPoint(x, y float32) (float32, float32) {
	return a.affine.TransformPoint(x, y)
}

// BuildEdgesFromScenePath builds edges from a scene.Path using the raster.EdgeBuilder.
// This bridges the scene package types to raster package interfaces.
func BuildEdgesFromScenePath(eb *raster.EdgeBuilder, path *scene.Path, transform scene.Affine) {
	eb.BuildFromPath(&scenePathAdapter{path: path}, sceneAffineAdapter{affine: transform})
}
