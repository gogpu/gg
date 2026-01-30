// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// Package ggcanvas provides seamless integration between gg 2D graphics and
// gogpu GPU-accelerated windows.
//
// This package enables drawing 2D UI elements directly in GPU-accelerated windows
// by managing the CPU-to-GPU pipeline automatically. The data flow is:
//
//	gg.Context (draw) -> Pixmap (CPU) -> GPU Texture -> Window
//
// # Architecture
//
// Canvas wraps a gg.Context and manages the texture upload pipeline:
//
//   - Draw operations use the familiar gg API
//   - Flush() uploads pixel data to GPU texture
//   - RenderTo() draws the texture to a gogpu window
//
// # Usage
//
// Basic usage with gogpu:
//
//	canvas := ggcanvas.New(app.GPUContextProvider(), 800, 600)
//	defer canvas.Close()
//
//	// Draw with gg API
//	cc := canvas.Context()
//	cc.SetRGB(1, 0, 0)
//	cc.DrawCircle(400, 300, 100)
//	cc.Fill()
//
//	// Render to gogpu window
//	canvas.RenderTo(dc)
//
// # Thread Safety
//
// Canvas is NOT safe for concurrent use. Create one Canvas per goroutine,
// or use external synchronization.
//
// # Performance Notes
//
//   - Texture is created lazily on first Flush()
//   - Dirty tracking avoids unnecessary GPU uploads
//   - Consider canvas size vs window size for optimal performance
//
// # Integration Without Circular Imports
//
// This package uses interfaces to avoid importing gogpu directly:
//
//   - gpucontext.DeviceProvider for device access
//   - Local interfaces for texture creation and drawing
//
// This allows gg to provide integration without creating circular dependencies.
package ggcanvas
