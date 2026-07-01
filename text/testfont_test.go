package text

import (
	"os"
	"testing"
)

// testFontData holds Go Regular font data for tests.
// This replaces goregular.TTF from golang.org/x/image/font/gofont/goregular.
// The font file is stored in testdata/goregular.ttf (148K, Go license).
var testFontData []byte

func init() {
	var err error
	testFontData, err = os.ReadFile("testdata/goregular.ttf")
	if err != nil {
		// Fallback: try from parent directory (for tests run from subdirs).
		testFontData, err = os.ReadFile("text/testdata/goregular.ttf")
		if err != nil {
			// Will cause tests to skip/fail if font is needed.
			testFontData = nil
		}
	}
}

// requireTestFont returns the test font data, skipping the test if unavailable.
func requireTestFont(t testing.TB) []byte {
	t.Helper()
	if testFontData == nil {
		t.Skip("test font (goregular.ttf) not available")
	}
	return testFontData
}
