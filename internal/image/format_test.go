package image

import "testing"

func TestFormat_BytesPerPixel(t *testing.T) {
	tests := []struct {
		format   Format
		expected int
	}{
		{FormatGray8, 1},
		{FormatGray16, 2},
		{FormatRGB8, 3},
		{FormatRGBA8, 4},
		{FormatRGBAPremul, 4},
		{FormatBGRA8, 4},
		{FormatBGRAPremul, 4},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.BytesPerPixel(); got != tt.expected {
				t.Errorf("BytesPerPixel() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestFormat_Channels(t *testing.T) {
	tests := []struct {
		format   Format
		expected int
	}{
		{FormatGray8, 1},
		{FormatGray16, 1},
		{FormatRGB8, 3},
		{FormatRGBA8, 4},
		{FormatRGBAPremul, 4},
		{FormatBGRA8, 4},
		{FormatBGRAPremul, 4},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.Channels(); got != tt.expected {
				t.Errorf("Channels() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestFormat_HasAlpha(t *testing.T) {
	tests := []struct {
		format   Format
		expected bool
	}{
		{FormatGray8, false},
		{FormatGray16, false},
		{FormatRGB8, false},
		{FormatRGBA8, true},
		{FormatRGBAPremul, true},
		{FormatBGRA8, true},
		{FormatBGRAPremul, true},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.HasAlpha(); got != tt.expected {
				t.Errorf("HasAlpha() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormat_IsPremultiplied(t *testing.T) {
	tests := []struct {
		format   Format
		expected bool
	}{
		{FormatGray8, false},
		{FormatGray16, false},
		{FormatRGB8, false},
		{FormatRGBA8, false},
		{FormatRGBAPremul, true},
		{FormatBGRA8, false},
		{FormatBGRAPremul, true},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.IsPremultiplied(); got != tt.expected {
				t.Errorf("IsPremultiplied() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormat_IsGrayscale(t *testing.T) {
	tests := []struct {
		format   Format
		expected bool
	}{
		{FormatGray8, true},
		{FormatGray16, true},
		{FormatRGB8, false},
		{FormatRGBA8, false},
		{FormatRGBAPremul, false},
		{FormatBGRA8, false},
		{FormatBGRAPremul, false},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.IsGrayscale(); got != tt.expected {
				t.Errorf("IsGrayscale() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormat_BitsPerChannel(t *testing.T) {
	tests := []struct {
		format   Format
		expected int
	}{
		{FormatGray8, 8},
		{FormatGray16, 16},
		{FormatRGB8, 8},
		{FormatRGBA8, 8},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.BitsPerChannel(); got != tt.expected {
				t.Errorf("BitsPerChannel() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestFormat_String(t *testing.T) {
	tests := []struct {
		format   Format
		expected string
	}{
		{FormatGray8, "Gray8"},
		{FormatGray16, "Gray16"},
		{FormatRGB8, "RGB8"},
		{FormatRGBA8, "RGBA8"},
		{FormatRGBAPremul, "RGBAPremul"},
		{FormatBGRA8, "BGRA8"},
		{FormatBGRAPremul, "BGRAPremul"},
		{Format(255), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.format.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestFormat_IsValid(t *testing.T) {
	tests := []struct {
		format   Format
		expected bool
	}{
		{FormatGray8, true},
		{FormatRGBA8, true},
		{FormatBGRAPremul, true},
		{Format(255), false},
		{formatCount, false},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.IsValid(); got != tt.expected {
				t.Errorf("IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormat_RowBytes(t *testing.T) {
	tests := []struct {
		format   Format
		width    int
		expected int
	}{
		{FormatGray8, 100, 100},
		{FormatGray16, 100, 200},
		{FormatRGB8, 100, 300},
		{FormatRGBA8, 100, 400},
		{FormatRGBA8, 1920, 7680},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.RowBytes(tt.width); got != tt.expected {
				t.Errorf("RowBytes(%d) = %d, want %d", tt.width, got, tt.expected)
			}
		})
	}
}

func TestFormat_ImageBytes(t *testing.T) {
	tests := []struct {
		format   Format
		width    int
		height   int
		expected int
	}{
		{FormatGray8, 100, 100, 10000},
		{FormatRGBA8, 100, 100, 40000},
		{FormatRGBA8, 1920, 1080, 8294400},  // Full HD
		{FormatRGBA8, 3840, 2160, 33177600}, // 4K
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.ImageBytes(tt.width, tt.height); got != tt.expected {
				t.Errorf("ImageBytes(%d, %d) = %d, want %d", tt.width, tt.height, got, tt.expected)
			}
		})
	}
}

func TestFormat_PremultipliedVersion(t *testing.T) {
	tests := []struct {
		format   Format
		expected Format
	}{
		{FormatGray8, FormatGray8},
		{FormatRGB8, FormatRGB8},
		{FormatRGBA8, FormatRGBAPremul},
		{FormatRGBAPremul, FormatRGBAPremul},
		{FormatBGRA8, FormatBGRAPremul},
		{FormatBGRAPremul, FormatBGRAPremul},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.PremultipliedVersion(); got != tt.expected {
				t.Errorf("PremultipliedVersion() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestFormat_UnpremultipliedVersion(t *testing.T) {
	tests := []struct {
		format   Format
		expected Format
	}{
		{FormatGray8, FormatGray8},
		{FormatRGB8, FormatRGB8},
		{FormatRGBA8, FormatRGBA8},
		{FormatRGBAPremul, FormatRGBA8},
		{FormatBGRA8, FormatBGRA8},
		{FormatBGRAPremul, FormatBGRA8},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			if got := tt.format.UnpremultipliedVersion(); got != tt.expected {
				t.Errorf("UnpremultipliedVersion() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestFormat_Info_InvalidFormat(t *testing.T) {
	invalid := Format(255)
	info := invalid.Info()

	if info.BytesPerPixel != 0 {
		t.Errorf("Invalid format Info().BytesPerPixel = %d, want 0", info.BytesPerPixel)
	}
}
