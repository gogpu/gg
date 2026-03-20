package gg

import (
	"testing"

	"github.com/gogpu/gg/text"
)

func TestLCDLayoutConstants(t *testing.T) {
	// Verify re-exported constants match the text package values.
	tests := []struct {
		name string
		got  LCDLayout
		want text.LCDLayout
	}{
		{"None", LCDLayoutNone, text.LCDLayoutNone},
		{"RGB", LCDLayoutRGB, text.LCDLayoutRGB},
		{"BGR", LCDLayoutBGR, text.LCDLayoutBGR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("LCDLayout%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestSetLCDLayout_NoAccelerator(t *testing.T) {
	// SetLCDLayout should not panic when no accelerator is registered.
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()
	dc.SetLCDLayout(LCDLayoutRGB)
	dc.SetLCDLayout(LCDLayoutBGR)
	dc.SetLCDLayout(LCDLayoutNone)
}

func TestLCDLayoutType(t *testing.T) {
	// LCDLayout is a type alias for text.LCDLayout, so values are interchangeable.
	// This test verifies the alias works at compile time and runtime.
	layout := LCDLayoutRGB
	if layout != text.LCDLayoutRGB {
		t.Errorf("LCDLayoutRGB = %d, want text.LCDLayoutRGB (%d)",
			layout, text.LCDLayoutRGB)
	}
}
