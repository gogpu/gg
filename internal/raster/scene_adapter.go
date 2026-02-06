// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

// ScenePathAdapter adapts a scene-like path to the core.PathLike interface.
// This allows EdgeBuilder to work with any path implementation without
// creating an import cycle.
//
// Usage:
//
//	// In a package that can import scene:
//	type scenePathWrapper struct {
//	    path *scene.Path
//	}
//	func (w *scenePathWrapper) IsEmpty() bool { return w.path.IsEmpty() }
//	func (w *scenePathWrapper) Verbs() []core.PathVerb {
//	    // Convert scene.PathVerb to core.PathVerb
//	    return convertVerbs(w.path.Verbs())
//	}
//	func (w *scenePathWrapper) Points() []float32 { return w.path.Points() }
//
// Then create an EdgeBuilder and use:
//
//	eb.BuildFromPath(&scenePathWrapper{path}, core.IdentityTransform{})
type ScenePathAdapter struct {
	// isEmpty reports if the path is empty.
	isEmpty bool

	// verbs is the verb stream in core format.
	verbs []PathVerb

	// points is the point data stream.
	points []float32
}

// NewScenePathAdapter creates a new adapter from verb and point data.
//
// This is a low-level constructor. Higher-level packages should provide
// convenience functions that convert from their path types.
func NewScenePathAdapter(isEmpty bool, verbs []PathVerb, points []float32) *ScenePathAdapter {
	return &ScenePathAdapter{
		isEmpty: isEmpty,
		verbs:   verbs,
		points:  points,
	}
}

// IsEmpty returns true if the path has no commands.
func (a *ScenePathAdapter) IsEmpty() bool {
	return a.isEmpty
}

// Verbs returns the verb stream.
func (a *ScenePathAdapter) Verbs() []PathVerb {
	return a.verbs
}

// Points returns the point data stream.
func (a *ScenePathAdapter) Points() []float32 {
	return a.points
}

// Verify that ScenePathAdapter implements PathLike.
var _ PathLike = (*ScenePathAdapter)(nil)
