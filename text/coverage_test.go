package text

import (
	"testing"

	"golang.org/x/image/font"
)

func TestMapHinting(t *testing.T) {
	tests := []struct {
		name string
		h    Hinting
		want font.Hinting
	}{
		{"none", HintingNone, font.HintingNone},
		{"vertical", HintingVertical, font.HintingVertical},
		{"full", HintingFull, font.HintingFull},
		{"unknown defaults to full", Hinting(99), font.HintingFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapHinting(tt.h)
			if got != tt.want {
				t.Errorf("mapHinting(%d) = %d, want %d", tt.h, got, tt.want)
			}
		})
	}
}

func TestDirectionMismatchError(t *testing.T) {
	err := &DirectionMismatchError{
		Index:    1,
		Got:      DirectionRTL,
		Expected: DirectionLTR,
	}
	s := err.Error()
	if s == "" {
		t.Error("Error() returned empty string")
	}
}
