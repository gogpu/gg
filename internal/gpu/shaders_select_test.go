package gpu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/naga/valid"
	"github.com/gogpu/naga/wgsl"
)

// TestWGSLShadersValidate parses and validates every gg WGSL shader with naga. Since naga v0.21 the validator
// enforces the WGSL rule that a select() condition is bool or vecN<bool> (SelectConditionNotABool, as Rust naga's
// valid/expression.rs does), which is the rule blend_soft_light broke when it used step() as the condition
// (P1-BLEND-003): SPIR-V converted it with != 0, HLSL emitted a vector ternary DXC rejects, and DXIL was invalid.
func TestWGSLShadersValidate(t *testing.T) {
	var files []string
	for _, pattern := range []string{"shaders/*.wgsl", "tilecompute/shaders/*.wgsl"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(matches) == 0 {
			t.Fatalf("no WGSL files match %s", pattern)
		}
		files = append(files, matches...)
	}

	for _, file := range files {
		t.Run(filepath.ToSlash(file), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateWGSL(string(src)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestWGSLValidateRejectsNonBoolSelect proves the check above can fail: a select() with a u32 condition must be
// rejected by the validator, or TestWGSLShadersValidate would no longer cover P1-BLEND-003.
func TestWGSLValidateRejectsNonBoolSelect(t *testing.T) {
	const src = `
@fragment
fn fs_main() -> @location(0) vec4<f32> {
    let c: u32 = 1u;
    return select(vec4<f32>(0.0), vec4<f32>(1.0), c);
}
`
	if err := validateWGSL(src); err == nil {
		t.Fatal("validator accepted select() with a u32 condition; want SelectConditionNotABool")
	}
}

func validateWGSL(src string) error {
	module, _, err := wgsl.Parse(src, wgsl.ParseOptions{})
	if err != nil {
		return err
	}
	_, err = valid.Validate(module, valid.Options{})
	return err
}
