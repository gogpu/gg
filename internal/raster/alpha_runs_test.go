// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// TestCatchOverflow tests the overflow clamping function.
func TestCatchOverflow(t *testing.T) {
	tests := []struct {
		input    uint16
		expected uint8
	}{
		{0, 0},
		{128, 128},
		{255, 255},
		{256, 255}, // Overflow case
		{300, 255}, // Overflow case
	}

	for _, tt := range tests {
		result := catchOverflow(tt.input)
		if result != tt.expected {
			t.Errorf("catchOverflow(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}
