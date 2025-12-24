// Package msdf provides Multi-channel Signed Distance Field generation
// for high-quality, scalable text rendering on GPU.
//
// MSDF (Multi-channel Signed Distance Field) is a technique that encodes
// glyph shape information into RGB texture channels. Unlike traditional SDF
// which uses a single distance value, MSDF preserves sharp corners by encoding
// directional distance information in separate channels.
//
// # How MSDF Works
//
// 1. Parse glyph outline into closed contours
// 2. Classify edge segments (line, quadratic, cubic)
// 3. Assign colors (RGB) to edges based on corner angles
// 4. For each pixel, find minimum signed distance to each color channel
// 5. Encode distances as RGB values (0.5 = on edge)
//
// The median of RGB channels recovers the accurate signed distance for
// anti-aliased rendering. This approach maintains crisp edges even when
// the texture is scaled significantly.
//
// # Usage
//
//	config := msdf.DefaultConfig()
//	config.Size = 64 // 64x64 pixel MSDF texture
//
//	generator := msdf.NewGenerator(config)
//	result, err := generator.Generate(outline)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// result.Data contains RGB pixel data
//	// Use in GPU shader with median function
//
// # WGSL Shader Example
//
//	fn median3(v: vec3<f32>) -> f32 {
//	    return max(min(v.r, v.g), min(max(v.r, v.g), v.b));
//	}
//
//	@fragment
//	fn fs_main(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
//	    let msdf = textureSample(msdf_tex, samp, uv).rgb;
//	    let sd = median3(msdf) - 0.5;
//	    let alpha = clamp(sd * px_range / length(fwidth(uv)) + 0.5, 0.0, 1.0);
//	    return vec4<f32>(color.rgb, color.a * alpha);
//	}
//
// # References
//
// - msdf-atlas-gen: https://github.com/Chlumsky/msdf-atlas-gen
// - MSDF paper: "Shape Decomposition for Multi-channel Distance Fields"
package msdf
