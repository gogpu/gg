package emoji

import (
	"errors"
	"testing"
)

func TestBitmapFormat_String(t *testing.T) {
	tests := []struct {
		format BitmapFormat
		want   string
	}{
		{FormatPNG, "PNG"},
		{FormatJPEG, "JPEG"},
		{FormatTIFF, "TIFF"},
		{FormatDUPE, "DUPE"},
		{FormatRaw, "Raw"},
		{BitmapFormat(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.format.String()
			if got != tt.want {
				t.Errorf("BitmapFormat(%d).String() = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestBitmapGlyph_Decode_UnsupportedFormat(t *testing.T) {
	glyph := &BitmapGlyph{
		Format: FormatTIFF,
		Data:   []byte{0, 1, 2, 3},
	}

	_, err := glyph.Decode()
	if !errors.Is(err, ErrUnsupportedBitmapFormat) {
		t.Errorf("Decode() error = %v, want ErrUnsupportedBitmapFormat", err)
	}
}

func TestBitmapGlyph_Decode_JPEG(t *testing.T) {
	glyph := &BitmapGlyph{
		Format: FormatJPEG,
		Data:   []byte{0, 1, 2, 3},
	}

	_, err := glyph.Decode()
	if err == nil {
		t.Error("Decode() should return error for JPEG (not implemented)")
	}
}

func TestBitmapGlyph_Decode_InvalidPNG(t *testing.T) {
	glyph := &BitmapGlyph{
		Format: FormatPNG,
		Data:   []byte{0, 1, 2, 3}, // Not valid PNG
	}

	_, err := glyph.Decode()
	if err == nil {
		t.Error("Decode() should return error for invalid PNG data")
	}
}

func TestNewSBIXParser_NoData(t *testing.T) {
	_, err := NewSBIXParser(nil, 100)
	if !errors.Is(err, ErrNoSBIXTable) {
		t.Errorf("NewSBIXParser(nil) error = %v, want ErrNoSBIXTable", err)
	}
}

func TestNewSBIXParser_TooShort(t *testing.T) {
	_, err := NewSBIXParser([]byte{0, 1, 2}, 100)
	if !errors.Is(err, ErrInvalidSBIXData) {
		t.Errorf("NewSBIXParser(short) error = %v, want ErrInvalidSBIXData", err)
	}
}

func TestParseGraphicType(t *testing.T) {
	tests := []struct {
		tag     string
		format  BitmapFormat
		wantErr bool
	}{
		{"png ", FormatPNG, false},
		{"jpg ", FormatJPEG, false},
		{"tiff", FormatTIFF, false},
		{"dupe", FormatDUPE, false},
		{"xxxx", FormatRaw, true},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			format, err := parseGraphicType(tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGraphicType(%q) error = %v, wantErr %v", tt.tag, err, tt.wantErr)
			}
			if format != tt.format {
				t.Errorf("parseGraphicType(%q) = %v, want %v", tt.tag, format, tt.format)
			}
		})
	}
}

func TestAbsDiff(t *testing.T) {
	tests := []struct {
		a, b, want uint16
	}{
		{10, 5, 5},
		{5, 10, 5},
		{10, 10, 0},
		{0, 100, 100},
	}

	for _, tt := range tests {
		got := absDiff(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("absDiff(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNewCBDTParser(t *testing.T) {
	tests := []struct {
		name     string
		cbdtData []byte
		cblcData []byte
		wantErr  bool
	}{
		{
			name:     "no CBDT",
			cbdtData: nil,
			cblcData: []byte{1, 2, 3},
			wantErr:  true,
		},
		{
			name:     "no CBLC",
			cbdtData: []byte{1, 2, 3},
			cblcData: nil,
			wantErr:  true,
		},
		{
			name:     "both present",
			cbdtData: []byte{1, 2, 3},
			cblcData: []byte{4, 5, 6},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewCBDTParser(tt.cbdtData, tt.cblcData)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCBDTParser() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && p == nil {
				t.Error("NewCBDTParser() returned nil parser without error")
			}
		})
	}
}

func TestCBDTParser_HasTable(t *testing.T) {
	p := &CBDTParser{
		cbdtData: []byte{1, 2, 3},
		cblcData: []byte{4, 5, 6},
	}

	if !p.HasTable() {
		t.Error("HasTable() = false, want true")
	}

	empty := &CBDTParser{}
	if empty.HasTable() {
		t.Error("empty HasTable() = true, want false")
	}
}
