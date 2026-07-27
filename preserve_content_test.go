package gg

import (
	"testing"
)

// TestPreserveContent_DefaultFalse verifies zero value is false (backward compat).
func TestPreserveContent_DefaultFalse(t *testing.T) {
	var target GPURenderTarget
	if target.PreserveContent {
		t.Error("PreserveContent should default to false")
	}
}

// TestPreserveContent_SetTrue verifies the flag can be set.
func TestPreserveContent_SetTrue(t *testing.T) {
	target := GPURenderTarget{PreserveContent: true}
	if !target.PreserveContent {
		t.Error("PreserveContent should be true when set")
	}
}

// TestPreserveContent_DoesNotAffectExistingFields verifies no struct layout breakage.
func TestPreserveContent_DoesNotAffectExistingFields(t *testing.T) {
	target := GPURenderTarget{
		ViewWidth:       800,
		ViewHeight:      600,
		PreserveContent: true,
		Width:           800,
		Height:          600,
		Stride:          3200,
	}
	if target.ViewWidth != 800 || target.ViewHeight != 600 {
		t.Error("PreserveContent field broke existing fields")
	}
	if target.Width != 800 || target.Height != 600 || target.Stride != 3200 {
		t.Error("PreserveContent field broke readback fields")
	}
}
