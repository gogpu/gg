package gpu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/ir"
)

// TestWGSLSelectConditionIsBool checks every gg WGSL shader for the WGSL rule that a select() condition is bool or
// vecN<bool>. Rust naga rejects anything else (valid/expression.rs, SelectConditionNotABool), so a shader that breaks
// it fails under -tags rust and in browsers. Our naga still accepts it, and its backends disagree on the result: SPIR-V
// converts with != 0, HLSL emits a vector ternary that DXC rejects, DXIL is invalid (P1-BLEND-003: blend_soft_light
// used step() as the condition). Once naga's validator enforces the rule (naga P1-WGSL-095), compiling the shaders
// covers it and this check can go.
func TestWGSLSelectConditionIsBool(t *testing.T) {
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
			ast, err := naga.Parse(string(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			module, err := naga.Lower(ast)
			if err != nil {
				t.Fatalf("lower: %v", err)
			}
			for i := range module.Functions {
				checkSelectConditions(t, module, &module.Functions[i])
			}
			for i := range module.EntryPoints {
				checkSelectConditions(t, module, &module.EntryPoints[i].Function)
			}
		})
	}
}

func checkSelectConditions(t *testing.T, module *ir.Module, fn *ir.Function) {
	t.Helper()
	for _, expr := range fn.Expressions {
		sel, ok := expr.Kind.(ir.ExprSelect)
		if !ok {
			continue
		}
		res, err := ir.ResolveExpressionType(module, fn, sel.Condition)
		if err != nil {
			t.Errorf("fn %s: resolve select condition: %v", fn.Name, err)
			continue
		}
		inner := res.Value
		if res.Handle != nil {
			inner = module.Types[*res.Handle].Inner
		}
		if !isBoolOrBoolVector(inner) {
			t.Errorf("fn %s: select() condition has type %#v, want bool or vecN<bool>", fn.Name, inner)
		}
	}
}

func isBoolOrBoolVector(inner ir.TypeInner) bool {
	switch v := inner.(type) {
	case ir.ScalarType:
		return v.Kind == ir.ScalarBool
	case ir.VectorType:
		return v.Scalar.Kind == ir.ScalarBool
	}
	return false
}
