// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// PTCL (Per-Tile Command List) command types and encoding.
// Port of Vello vello_shaders/src/shared/ptcl.rs.
//
// Each tile in the tile grid has its own PTCL that drives fine rasterization.
// Commands are encoded as a stream of uint32 words: [tag, payload...].
//
// Reference: vello_shaders/src/shared/ptcl.rs

package tilecompute

import "math"

// PTCL command tags (from Vello vello_shaders/src/shared/ptcl.rs).
const (
	CmdEnd       uint32 = 0  // End of command list for this tile
	CmdFill      uint32 = 1  // Compute area coverage from segments
	CmdSolid     uint32 = 3  // Fully covered tile (backdrop != 0, no segments)
	CmdColor     uint32 = 5  // Apply solid color
	CmdBeginClip uint32 = 10 // Begin clip layer
	CmdEndClip   uint32 = 11 // End clip layer + composite
)

// ptclInitialAlloc is the initial capacity (in uint32s) for a PTCL.
const ptclInitialAlloc = 64

// CmdFillData is the payload for CmdFill.
// Encoded as 3 uint32 after the tag:
//
//	[0] = (segCount << 1) | evenOddFlag
//	[1] = segIndex (into PathSegment buffer)
//	[2] = backdrop (winding at left edge, stored as uint32 bitcast of int32)
type CmdFillData struct {
	SegCount uint32
	EvenOdd  bool
	SegIndex uint32
	Backdrop int32
}

// CmdColorData is the payload for CmdColor.
// Encoded as 1 uint32 after the tag: packed premultiplied RGBA.
type CmdColorData struct {
	RGBA uint32 // Packed: R | (G << 8) | (B << 16) | (A << 24)
}

// CmdEndClipData is the payload for CmdEndClip.
// Encoded as 2 uint32 after the tag:
//
//	[0] = blend mode (0 = normal source-over)
//	[1] = alpha as float32 bits
type CmdEndClipData struct {
	Blend uint32
	Alpha float32
}

// PTCL is a Per-Tile Command List.
// Commands are encoded as a flat stream of uint32 words.
type PTCL struct {
	Cmds []uint32 // Raw command stream
}

// NewPTCL creates a new PTCL with initial capacity.
func NewPTCL() *PTCL {
	return &PTCL{
		Cmds: make([]uint32, 0, ptclInitialAlloc),
	}
}

// WriteFill writes a CmdFill command.
// Payload: [(segCount << 1) | evenOdd, segIndex, backdrop_as_uint32]
func (p *PTCL) WriteFill(segCount uint32, evenOdd bool, segIndex uint32, backdrop int32) {
	var evenOddFlag uint32
	if evenOdd {
		evenOddFlag = 1
	}
	//nolint:gosec // Intentional int32 to uint32 bitcast for backdrop encoding
	p.Cmds = append(p.Cmds, CmdFill, (segCount<<1)|evenOddFlag, segIndex, uint32(backdrop))
}

// WriteSolid writes a CmdSolid command (no payload).
func (p *PTCL) WriteSolid() {
	p.Cmds = append(p.Cmds, CmdSolid)
}

// WriteColor writes a CmdColor command.
// Payload: [packed premultiplied RGBA]
func (p *PTCL) WriteColor(rgba uint32) {
	p.Cmds = append(p.Cmds, CmdColor, rgba)
}

// WriteEnd writes CmdEnd to terminate the list.
func (p *PTCL) WriteEnd() {
	p.Cmds = append(p.Cmds, CmdEnd)
}

// WriteBeginClip writes a CmdBeginClip command (no payload).
func (p *PTCL) WriteBeginClip() {
	p.Cmds = append(p.Cmds, CmdBeginClip)
}

// WriteEndClip writes a CmdEndClip command.
// Payload: [blend mode, alpha_as_float32_bits]
func (p *PTCL) WriteEndClip(blend uint32, alpha float32) {
	p.Cmds = append(p.Cmds, CmdEndClip, blend, math.Float32bits(alpha))
}

// ReadCmd reads a command at the given offset and returns the tag and next offset.
// Returns CmdEnd and same offset if at end of stream.
func (p *PTCL) ReadCmd(offset int) (tag uint32, nextOffset int) {
	if offset >= len(p.Cmds) {
		return CmdEnd, offset
	}
	tag = p.Cmds[offset]
	return tag, offset + 1
}

// ReadFillData reads CmdFill payload at the given offset.
// Returns the fill data and the offset past the payload.
func (p *PTCL) ReadFillData(offset int) (CmdFillData, int) {
	packed := p.Cmds[offset]
	segIndex := p.Cmds[offset+1]
	backdropBits := p.Cmds[offset+2]
	return CmdFillData{
		SegCount: packed >> 1,
		EvenOdd:  packed&1 != 0,
		SegIndex: segIndex,
		Backdrop: int32(backdropBits), //nolint:gosec // Intentional uint32 to int32 bitcast
	}, offset + 3
}

// ReadColorData reads CmdColor payload at the given offset.
// Returns the color data and the offset past the payload.
func (p *PTCL) ReadColorData(offset int) (CmdColorData, int) {
	return CmdColorData{RGBA: p.Cmds[offset]}, offset + 1
}

// ReadEndClipData reads CmdEndClip payload at the given offset.
// Returns the end-clip data and the offset past the payload.
func (p *PTCL) ReadEndClipData(offset int) (CmdEndClipData, int) {
	blend := p.Cmds[offset]
	alphaBits := p.Cmds[offset+1]
	return CmdEndClipData{
		Blend: blend,
		Alpha: math.Float32frombits(alphaBits),
	}, offset + 2
}
